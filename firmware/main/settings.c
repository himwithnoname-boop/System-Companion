#include "companion.h"
#include "nvs_flash.h"
#include "nvs.h"
settings_t settings={.version=3,.theme=0,.brightness=170,.rate=1,.animations=1,.clock_mode=0,.timeout=120,.auto_rotate=1};
void settings_load(void) {
    ESP_ERROR_CHECK(nvs_flash_init()); nvs_handle_t h;
    if(nvs_open("companion",NVS_READONLY,&h)==ESP_OK) {
        settings_t s=settings;size_t n=sizeof(s);
        if(nvs_get_blob(h,"display",&s,&n)==ESP_OK&&s.version>=1&&s.version<=3&&s.theme<5&&s.brightness>=20&&s.rate>=1&&s.rate<=5&&s.clock_mode<4) {
            if(s.version<3) s.center_mode=0;
            if(s.center_mode>=6) s.center_mode=0;
            s.version=3;settings=s;
        }
        nvs_close(h);
    }
}
void settings_save(void) {
    nvs_handle_t h; if(nvs_open("companion",NVS_READWRITE,&h)==ESP_OK) {
        bool saved=nvs_set_blob(h,"display",&settings,sizeof(settings))==ESP_OK&&nvs_commit(h)==ESP_OK;
        nvs_close(h);
        if(saved) usb_settings_report();
    }
}
