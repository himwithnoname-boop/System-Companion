#include "companion.h"
#include "orientation.h"
#include "driver/gpio.h"
#include "driver/i2c.h"
#include "driver/spi_master.h"
#include "esp_lcd_panel_io.h"
#include "esp_lcd_panel_ops.h"
#include "esp_lcd_sh8601.h"
#include "esp_heap_caps.h"
#include "esp_timer.h"
#include "read_lcd_id_bsp.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "esp_log.h"
static esp_lcd_panel_io_handle_t panel_io;
static esp_lcd_panel_handle_t panel;
static uint8_t panel_id;
static lv_disp_t *display;
static lv_indev_t *touch_input;
static int rotation=3,candidate=-1,stable;
static float filtered_x,filtered_y,filtered_z;
static int inject_x,inject_y,inject_state=-1;
static const sh8601_lcd_init_cmd_t sh_init[]={
    {0x11,(uint8_t[]){0},0,120},{0x44,(uint8_t[]){1,0xd1},2,0},{0x35,(uint8_t[]){0},1,0},
    {0x53,(uint8_t[]){0x20},1,10},{0x51,(uint8_t[]){0},1,10},{0x29,(uint8_t[]){0},0,10},{0x51,(uint8_t[]){170},1,0}};
static const sh8601_lcd_init_cmd_t co_init[]={
    {0x11,(uint8_t[]){0},0,80},{0xc4,(uint8_t[]){0x80},1,0},{0x53,(uint8_t[]){0x20},1,1},
    {0x63,(uint8_t[]){0xff},1,1},{0x51,(uint8_t[]){0},1,1},{0x29,(uint8_t[]){0},0,10},{0x51,(uint8_t[]){170},1,0}};
static bool flushed(esp_lcd_panel_io_handle_t io,esp_lcd_panel_io_event_data_t *e,void *ctx) { lv_disp_flush_ready(ctx);return false; }
static void flush(lv_disp_drv_t *d,const lv_area_t *a,lv_color_t *pixels) {
    int offset=panel_id==0x86?0:6;
    ESP_ERROR_CHECK(esp_lcd_panel_draw_bitmap(panel,a->x1+offset,a->y1,a->x2+offset+1,a->y2+1,pixels));
}
static void round_area(lv_disp_drv_t *d,lv_area_t *a) { a->x1&=~1;a->y1&=~1;a->x2|=1;a->y2|=1; }
static void touch(lv_indev_drv_t *drv,lv_indev_data_t *d) {
    if(inject_state>=0) { d->point.x=inject_x;d->point.y=inject_y;d->state=inject_state?LV_INDEV_STATE_PRESSED:LV_INDEV_STATE_RELEASED;return; }
    uint8_t reg=2,b[5]={0};
    if(i2c_master_write_read_device(I2C_NUM_0,0x38,&reg,1,b,5,pdMS_TO_TICKS(10))==ESP_OK&&(b[0]&0xf)>0) {
        int x=((b[1]&15)<<8)|b[2],y=((b[3]&15)<<8)|b[4];
        if(x<466&&y<466) {d->point.x=x;d->point.y=y;d->state=LV_INDEV_STATE_PRESSED;return;}
    } d->state=LV_INDEV_STATE_RELEASED;
}
void ui_test_touch(int x,int y,int state) {
    inject_x=x;inject_y=y;inject_state=state;
    if(touch_input) lv_timer_ready(touch_input->driver->read_timer);
}
static void tick(void *ctx) {lv_tick_inc(2);}
void board_brightness(uint8_t value) { if(panel_io) ESP_ERROR_CHECK_WITHOUT_ABORT(esp_lcd_panel_io_tx_param(panel_io,0x02005100,&value,1)); }
uint8_t board_panel_id(void) {return panel_id;}
int board_rotation(void) {return rotation*90;}
void board_prepare_sleep(void) {
    while(display->driver->draw_buf->flushing) vTaskDelay(pdMS_TO_TICKS(1));
    board_brightness(0);
    ESP_ERROR_CHECK_WITHOUT_ABORT(esp_lcd_panel_disp_on_off(panel,false));
    ESP_ERROR_CHECK_WITHOUT_ABORT(esp_lcd_panel_io_tx_param(panel_io,0x02001000,NULL,0));
    vTaskDelay(pdMS_TO_TICKS(120));
    uint8_t disable[]={8,0};i2c_master_write_to_device(I2C_NUM_0,0x6b,disable,2,pdMS_TO_TICKS(20));
    gpio_set_level(GPIO_NUM_42,0);gpio_hold_en(GPIO_NUM_42);gpio_deep_sleep_hold_en();
}
void board_auto_rotate(void) {
    if(!settings.auto_rotate) {stable=0;return;}
    float x,y,z;if(!board_imu(&x,&y,&z)) return;
    filtered_x=.7f*filtered_x+.3f*x;filtered_y=.7f*filtered_y+.3f*y;filtered_z=.7f*filtered_z+.3f*z;
    int target=orientation_target(filtered_x,filtered_y,filtered_z);
    if(target<0) {stable=0;return;}
    if(target!=candidate) {candidate=target;stable=1;return;}
    if(stable<6) stable++;
    if(stable<6||target==rotation) return;
    lv_indev_t *input=lv_indev_get_next(NULL);if(input&&input->proc.state==LV_INDEV_STATE_PRESSED) return;
    rotation=target;lv_disp_set_rotation(display,(lv_disp_rot_t)rotation);lv_obj_invalidate(lv_scr_act());stable=0;
}
bool board_imu(float *x,float *y,float *z) {
    uint8_t reg=0x35,b[6];if(i2c_master_write_read_device(I2C_NUM_0,0x6b,&reg,1,b,6,pdMS_TO_TICKS(10))!=ESP_OK) return false;
    *x=(int16_t)(b[0]|b[1]<<8)/4096.f;*y=(int16_t)(b[2]|b[3]<<8)/4096.f;*z=(int16_t)(b[4]|b[5]<<8)/4096.f;return true;
}
void board_init(void) {
    gpio_deep_sleep_hold_dis();gpio_hold_dis(GPIO_NUM_42);
    gpio_set_direction(GPIO_NUM_42,GPIO_MODE_OUTPUT);gpio_set_level(GPIO_NUM_42,1);
    panel_id=read_lcd_id();
    ESP_LOGI("board","panel=%02x PSRAM=%u",panel_id,(unsigned)heap_caps_get_total_size(MALLOC_CAP_SPIRAM));
    static lv_disp_drv_t drv;static lv_disp_draw_buf_t buf;
    spi_bus_config_t bus=SH8601_PANEL_BUS_QSPI_CONFIG(10,11,12,13,14,466*40*2);
    ESP_ERROR_CHECK(spi_bus_initialize(SPI2_HOST,&bus,SPI_DMA_CH_AUTO));
    esp_lcd_panel_io_spi_config_t io=SH8601_PANEL_IO_QSPI_CONFIG(9,flushed,&drv);
    io.pclk_hz=40000000;
    ESP_ERROR_CHECK(esp_lcd_new_panel_io_spi((esp_lcd_spi_bus_handle_t)SPI2_HOST,&io,&panel_io));
    sh8601_vendor_config_t vendor={.init_cmds=panel_id==0x86?sh_init:co_init,.init_cmds_size=panel_id==0x86?7:7,.flags.use_qspi_interface=1};
    esp_lcd_panel_dev_config_t cfg={.reset_gpio_num=21,.rgb_ele_order=LCD_RGB_ELEMENT_ORDER_RGB,.bits_per_pixel=16,.vendor_config=&vendor};
    ESP_ERROR_CHECK(esp_lcd_new_panel_sh8601(panel_io,&cfg,&panel));
    ESP_ERROR_CHECK(esp_lcd_panel_reset(panel));ESP_ERROR_CHECK(esp_lcd_panel_init(panel));
    ESP_ERROR_CHECK(esp_lcd_panel_disp_on_off(panel,true));board_brightness(settings.brightness);
    i2c_config_t i2c={.mode=I2C_MODE_MASTER,.sda_io_num=47,.scl_io_num=48,.sda_pullup_en=1,.scl_pullup_en=1,.master.clk_speed=400000};
    ESP_ERROR_CHECK(i2c_param_config(I2C_NUM_0,&i2c));ESP_ERROR_CHECK(i2c_driver_install(I2C_NUM_0,I2C_MODE_MASTER,0,0,0));
    uint8_t normal[]={0,0};i2c_master_write_to_device(I2C_NUM_0,0x38,normal,2,pdMS_TO_TICKS(50));
    uint8_t imu[]={2,0x60};i2c_master_write_to_device(I2C_NUM_0,0x6b,imu,2,pdMS_TO_TICKS(20));
    uint8_t accel[]={3,0x23};i2c_master_write_to_device(I2C_NUM_0,0x6b,accel,2,pdMS_TO_TICKS(20));
    uint8_t enable[]={8,1};i2c_master_write_to_device(I2C_NUM_0,0x6b,enable,2,pdMS_TO_TICKS(20));
    lv_init();
    lv_color_t *a=heap_caps_malloc(466*40*2,MALLOC_CAP_DMA|MALLOC_CAP_INTERNAL),*b=heap_caps_malloc(466*40*2,MALLOC_CAP_DMA|MALLOC_CAP_INTERNAL);
    assert(a&&b);lv_disp_draw_buf_init(&buf,a,b,466*40);lv_disp_drv_init(&drv);
    drv.hor_res=466;drv.ver_res=466;drv.flush_cb=flush;drv.rounder_cb=round_area;drv.draw_buf=&buf;
    drv.sw_rotate=1;drv.rotated=LV_DISP_ROT_270;lv_disp_t *disp=lv_disp_drv_register(&drv);display=disp;
    static lv_indev_drv_t input;lv_indev_drv_init(&input);input.type=LV_INDEV_TYPE_POINTER;input.read_cb=touch;input.disp=disp;touch_input=lv_indev_drv_register(&input);
    esp_timer_handle_t timer;esp_timer_create_args_t timer_cfg={.callback=tick,.name="lv_tick"};
    ESP_ERROR_CHECK(esp_timer_create(&timer_cfg,&timer));ESP_ERROR_CHECK(esp_timer_start_periodic(timer,2000));
}
