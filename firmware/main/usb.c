#include "companion.h"
#include "gpu_profiles.h"
static char active_host[33];
#include <stdio.h>
#include <string.h>
#include <stdlib.h>
#include <stdatomic.h>
#include "tinyusb.h"
#include "tusb_cdc_acm.h"
#include "esp_partition.h"
#include "esp_heap_caps.h"
#include "esp_system.h"
#include "esp_rom_sys.h"
#include "esp_mac.h"
#include "esp_timer.h"
#include "wear_levelling.h"
#include "driver/spi_master.h"
#include "driver/sdspi_host.h"
#include "sdmmc_cmd.h"
#include "soc/soc.h"
#include "soc/rtc_cntl_reg.h"
#include "cJSON.h"
#include "mbedtls/base64.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "freertos/queue.h"
#include "freertos/semphr.h"
static QueueHandle_t inbox;
static SemaphoreHandle_t tx_mutex;
static char frame[FRAME_MAX];static size_t frame_len;static bool overflow;
static char serial_number[20];
static unsigned accepted,rejected;
static atomic_bool capturing,stopping;
static const tusb_desc_device_t descriptor={
    .bLength=sizeof(tusb_desc_device_t),.bDescriptorType=TUSB_DESC_DEVICE,.bcdUSB=0x0200,
    .bDeviceClass=TUSB_CLASS_MISC,.bDeviceSubClass=MISC_SUBCLASS_COMMON,.bDeviceProtocol=MISC_PROTOCOL_IAD,
    .bMaxPacketSize0=64,.idVendor=0x303a,.idProduct=0x4002,.bcdDevice=0x0100,
    .iManufacturer=1,.iProduct=2,.iSerialNumber=3,.bNumConfigurations=1};
static const uint8_t configuration[]={
    TUD_CONFIG_DESCRIPTOR(1,3,0,TUD_CONFIG_DESC_LEN+TUD_CDC_DESC_LEN+TUD_MSC_DESC_LEN,0,500),
    TUD_CDC_DESCRIPTOR(0,4,0x81,8,0x02,0x82,64),
    TUD_MSC_DESCRIPTOR(2,5,0x03,0x83,64)};
static const char *strings[]={(const char[]){0x09,0x04},"System Companion","System Companion AMOLED",serial_number,"Telemetry","SYSTEM_MONITOR"};
void usb_send(const char *line) {
    if(stopping||!tx_mutex||!tud_cdc_n_connected(0)) return;
    if(xSemaphoreTake(tx_mutex,pdMS_TO_TICKS(250))!=pdTRUE) return;
    size_t len=strlen(line),pos=0;int64_t deadline=esp_timer_get_time()+1000000;
    while(!stopping&&pos<len&&esp_timer_get_time()<deadline&&tud_cdc_n_connected(0)) {
        size_t n=tinyusb_cdcacm_write_queue(0,(const uint8_t *)line+pos,len-pos);pos+=n;
        tinyusb_cdcacm_write_flush(0,pdMS_TO_TICKS(100));if(!n) vTaskDelay(pdMS_TO_TICKS(2));
    }xSemaphoreGive(tx_mutex);
}
void usb_action(const char *action) {char s[128];snprintf(s,sizeof(s),"{\"type\":\"action\",\"action\":\"%s\",\"interval\":%u}\n",action,settings.rate);usb_send(s);}
void usb_prepare_sleep(void) {
    stopping=true;tud_disconnect();
    while(capturing) vTaskDelay(pdMS_TO_TICKS(1));
    xSemaphoreTake(tx_mutex,portMAX_DELAY);
    ESP_ERROR_CHECK(tinyusb_driver_uninstall());
    xSemaphoreGive(tx_mutex);
}
static void rx(int itf,cdcacm_event_t *event) {
    uint8_t bytes[512];size_t n=0;
    while(tinyusb_cdcacm_read(itf,bytes,sizeof(bytes),&n)==ESP_OK&&n) {
        for(size_t i=0;i<n;i++) {uint8_t b=bytes[i];if(b=='\r')continue;
            if(b=='\n') {if(!overflow&&frame_len) {frame[frame_len]=0;char *copy=strdup(frame);if(copy&&xQueueSend(inbox,&copy,0)!=pdTRUE) free(copy);}else if(overflow) rejected++;frame_len=0;overflow=false;}
            else if(!overflow) {if(frame_len<FRAME_MAX-1) frame[frame_len++]=b;else overflow=true;}
        }
    }
}
static cJSON *preferences(void) {
    cJSON *o=cJSON_CreateObject();
    cJSON_AddNumberToObject(o,"theme",settings.theme);
    cJSON_AddNumberToObject(o,"brightness",settings.brightness);
    cJSON_AddBoolToObject(o,"fahrenheit",settings.fahrenheit);
    cJSON_AddBoolToObject(o,"auto_rotate",settings.auto_rotate);
    cJSON_AddNumberToObject(o,"center_mode",settings.center_mode);
    cJSON_AddNumberToObject(o,"timeout",settings.timeout);
    cJSON_AddNumberToObject(o,"page",ui_page());
    cJSON_AddBoolToObject(o,"gpu_selection",true);
    cJSON_AddStringToObject(o,"host_key",active_host);
    cJSON_AddItemToObject(o,"gpu_hidden",gpu_profile_get(active_host));
    return o;
}
static void send_object(cJSON *o) {
    char *json=cJSON_PrintUnformatted(o);
    if(json) {size_t n=strlen(json);char *line=malloc(n+2);if(line) {memcpy(line,json,n);line[n]='\n';line[n+1]=0;usb_send(line);free(line);}free(json);}
    cJSON_Delete(o);
}
void usb_settings_report(void) {
    if(!tx_mutex||!tud_cdc_n_connected(0)) return;
    cJSON *o=cJSON_CreateObject();cJSON_AddStringToObject(o,"type","settings");
    cJSON_AddItemToObject(o,"settings",preferences());send_object(o);
}
static void hello(void) {
    cJSON *o=cJSON_CreateObject();cJSON_AddStringToObject(o,"type","hello");
    cJSON_AddStringToObject(o,"device","system-companion");cJSON_AddNumberToObject(o,"v",1);
    cJSON_AddStringToObject(o,"firmware",COMPANION_VERSION);cJSON_AddNumberToObject(o,"width",466);cJSON_AddNumberToObject(o,"height",466);
    cJSON_AddStringToObject(o,"serial",serial_number);cJSON_AddNumberToObject(o,"panel",board_panel_id());
    cJSON_AddStringToObject(o,"storage","flash");cJSON_AddBoolToObject(o,"sd_ready",storage_sd_ready());
    cJSON_AddStringToObject(o,"sd_name","MoniStore");cJSON_AddStringToObject(o,"sd_label",storage_sd_label());
    cJSON_AddNumberToObject(o,"sd_bytes",storage_sd_bytes());cJSON_AddItemToObject(o,"settings",preferences());send_object(o);
}
void usb_poll(void) {
    char *line;while(xQueueReceive(inbox,&line,0)==pdTRUE) {
        if(model_accept(line,strlen(line))) {accepted++;free(line);continue;}
        cJSON *o=cJSON_Parse(line);free(line);if(!o) {rejected++;continue;}
        cJSON *kind=cJSON_GetObjectItemCaseSensitive(o,"type"),*v=cJSON_GetObjectItemCaseSensitive(o,"v");
        if(!cJSON_IsString(kind)||!cJSON_IsNumber(v)||v->valueint!=1) {rejected++;cJSON_Delete(o);continue;}
        const char *type=kind->valuestring;
        if(!strcmp(type,"hello")) {
            cJSON *host=cJSON_GetObjectItemCaseSensitive(o,"host_key");
            if(cJSON_IsString(host)&&gpu_profile_key(host->valuestring)) strcpy(active_host,host->valuestring);
            else active_host[0]=0;
        }
        if(!strcmp(type,"settings")) {
            cJSON *hidden=cJSON_GetObjectItemCaseSensitive(o,"gpu_hidden");
            if(hidden) {
                cJSON *host=cJSON_GetObjectItemCaseSensitive(o,"host_key");
                if(!cJSON_IsString(host)||!gpu_profile_set(host->valuestring,hidden)) {
                    usb_send("{\"type\":\"error\",\"error\":\"Cannot save GPU selection\"}\n");
                    cJSON_Delete(o);continue;
                }
                strcpy(active_host,host->valuestring);
            }
        }
        if(!strcmp(type,"settings")) {
            cJSON *rotate=cJSON_GetObjectItemCaseSensitive(o,"auto_rotate");
            if(cJSON_IsBool(rotate)) settings.auto_rotate=cJSON_IsTrue(rotate);
            cJSON *center=cJSON_GetObjectItemCaseSensitive(o,"center_mode");
            if(cJSON_IsNumber(center)) overview_select(center->valueint);
        }
        if(!strcmp(type,"hello")) hello();
        else if(!strcmp(type,"settings")) {
            cJSON *n=cJSON_GetObjectItemCaseSensitive(o,"theme");if(cJSON_IsNumber(n)&&n->valueint>=0&&n->valueint<5) settings.theme=n->valueint;
            n=cJSON_GetObjectItemCaseSensitive(o,"brightness");if(cJSON_IsNumber(n)&&n->valueint>=20&&n->valueint<=255) settings.brightness=n->valueint;
            n=cJSON_GetObjectItemCaseSensitive(o,"fahrenheit");if(cJSON_IsBool(n)) settings.fahrenheit=cJSON_IsTrue(n);
            n=cJSON_GetObjectItemCaseSensitive(o,"timeout");if(cJSON_IsNumber(n)&&n->valueint>=0&&n->valueint<=3600) settings.timeout=n->valueint;
            settings_save();board_brightness(settings.brightness);n=cJSON_GetObjectItemCaseSensitive(o,"page");
            ui_navigate(cJSON_IsNumber(n)&&n->valueint>=0&&n->valueint<9?n->valueint:ui_page());
            usb_settings_report();usb_send("{\"type\":\"ack\",\"action\":\"settings\"}\n");
        }else if(!strcmp(type,"capture")) ui_request_capture();
        else if(!strcmp(type,"bootloader")) {
            usb_send("{\"type\":\"ack\",\"action\":\"bootloader\"}\n");
            vTaskDelay(pdMS_TO_TICKS(150));REG_SET_BIT(RTC_CNTL_OPTION1_REG,RTC_CNTL_FORCE_DOWNLOAD_BOOT);esp_rom_software_reset_system();
        }
        else if(!strcmp(type,"diagnostics")) {
            telemetry_t t;model_copy(&t);float x=0,y=0,z=0;bool imu=board_imu(&x,&y,&z);
            cJSON *report=cJSON_CreateObject();cJSON_AddStringToObject(report,"type","diagnostics");
            cJSON_AddNumberToObject(report,"page",ui_page());cJSON_AddNumberToObject(report,"reset_reason",esp_reset_reason());
            cJSON_AddNumberToObject(report,"accepted",accepted);cJSON_AddNumberToObject(report,"rejected",rejected);cJSON_AddNumberToObject(report,"seq",t.seq);
            cJSON_AddNumberToObject(report,"heap",heap_caps_get_free_size(MALLOC_CAP_INTERNAL));cJSON_AddNumberToObject(report,"psram",heap_caps_get_free_size(MALLOC_CAP_SPIRAM));
            cJSON_AddNumberToObject(report,"uptime",esp_timer_get_time()/1000000);cJSON_AddBoolToObject(report,"imu",imu);
            float acceleration[3]={x,y,z};cJSON_AddItemToObject(report,"accel",cJSON_CreateFloatArray(acceleration,3));
            cJSON_AddNumberToObject(report,"rotation",board_rotation());cJSON_AddBoolToObject(report,"auto_rotate",settings.auto_rotate);
            cJSON_AddNumberToObject(report,"center_mode",overview_mode());cJSON_AddNumberToObject(report,"theme",settings.theme);cJSON_AddNumberToObject(report,"brightness",settings.brightness);
            cJSON_AddNumberToObject(report,"storage_bytes",storage_flash_bytes());cJSON_AddNumberToObject(report,"power_hold_ms",power_hold_ms());cJSON_AddNumberToObject(report,"wake_reason",power_wake_reason());
            cJSON_AddNumberToObject(report,"gpu_count",t.gpu_count);cJSON_AddNumberToObject(report,"gpu_detected",t.gpu_detected);cJSON_AddNumberToObject(report,"gpu_load_available",t.gpu_load_available);
            cJSON_AddNumberToObject(report,"gpu_load",t.gpu_load);send_object(report);

        }else if(!strcmp(type,"touch")) {
            cJSON *x=cJSON_GetObjectItemCaseSensitive(o,"x"),*y=cJSON_GetObjectItemCaseSensitive(o,"y"),*state=cJSON_GetObjectItemCaseSensitive(o,"state");
            if(cJSON_IsNumber(x)&&cJSON_IsNumber(y)&&cJSON_IsNumber(state)&&x->valueint>=0&&x->valueint<466&&y->valueint>=0&&y->valueint<466&&state->valueint>=-1&&state->valueint<=1) {ui_test_touch(x->valueint,y->valueint,state->valueint);cJSON_Delete(o);return;}
        }else rejected++;
        cJSON_Delete(o);
    }
}
static void capture_task(void *arg) {
    lv_img_dsc_t *img=arg;char s[1600];snprintf(s,sizeof(s),"{\"type\":\"frame_begin\",\"bytes\":%u,\"width\":%u,\"height\":%u}\n",(unsigned)img->data_size,img->header.w,img->header.h);usb_send(s);
    for(size_t pos=0;!stopping&&pos<img->data_size&&tud_cdc_n_connected(0);pos+=768) {
        unsigned char encoded[1032];size_t written=0,len=img->data_size-pos;if(len>768)len=768;
        mbedtls_base64_encode(encoded,sizeof(encoded),&written,img->data+pos,len);encoded[written]=0;
        snprintf(s,sizeof(s),"{\"type\":\"frame_data\",\"offset\":%u,\"data\":\"%s\"}\n",(unsigned)pos,encoded);usb_send(s);
    }usb_send("{\"type\":\"frame_end\"}\n");lv_snapshot_free(img);capturing=false;vTaskDelete(NULL);
}
void usb_capture(void) {
    if(stopping||capturing) return;
    lv_img_dsc_t *img=lv_snapshot_take(lv_scr_act(),LV_IMG_CF_TRUE_COLOR);
    if(img) {img->data_size=(uint32_t)img->header.w*img->header.h*sizeof(lv_color_t);capturing=true;if(xTaskCreate(capture_task,"capture",12288,img,2,NULL)!=pdPASS) {lv_snapshot_free(img);capturing=false;}}
}
void usb_init(void) {
    inbox=xQueueCreate(8,sizeof(char *));tx_mutex=xSemaphoreCreateMutex();assert(inbox&&tx_mutex);
    uint8_t mac[6];esp_read_mac(mac,ESP_MAC_WIFI_STA);snprintf(serial_number,sizeof(serial_number),"%02X%02X%02X%02X%02X%02X",mac[0],mac[1],mac[2],mac[3],mac[4],mac[5]);
    storage_init();
    tinyusb_config_t cfg={.device_descriptor=&descriptor,.string_descriptor=strings,.string_descriptor_count=6,.configuration_descriptor=configuration};
    ESP_ERROR_CHECK(tinyusb_driver_install(&cfg));
    tinyusb_config_cdcacm_t cdc={.usb_dev=TINYUSB_USBDEV_0,.cdc_port=TINYUSB_CDC_ACM_0,.callback_rx=rx};ESP_ERROR_CHECK(tusb_cdc_acm_init(&cdc));
}
