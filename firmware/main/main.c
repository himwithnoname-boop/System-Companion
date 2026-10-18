#include "companion.h"
#include "gpu_profiles.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
void app_main(void) {
    settings_load();gpu_profiles_load();model_init();board_init();ui_init();usb_init();power_init();
    while(1) { power_poll();usb_poll();uint32_t delay=lv_timer_handler();vTaskDelay(pdMS_TO_TICKS(delay<5?5:delay>33?33:delay)); }
}
