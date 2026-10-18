#include "companion.h"
#include "power_button.h"
#include "driver/gpio.h"
#include "driver/rtc_io.h"
#include "esp_sleep.h"
#include "esp_timer.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"

static power_button_t button;
static int shown=-1;

void power_init(void) {
    ESP_ERROR_CHECK(rtc_gpio_deinit(GPIO_NUM_0));
    gpio_config_t input={.pin_bit_mask=1ULL<<GPIO_NUM_0,.mode=GPIO_MODE_INPUT,
        .pull_up_en=GPIO_PULLUP_ENABLE,.pull_down_en=GPIO_PULLDOWN_DISABLE};
    ESP_ERROR_CHECK(gpio_config(&input));
}
static void power_off(void) {
    /* Each settings change is already committed; finish any current card I/O
     * before disconnecting. The computer must eject its own filesystem cache. */
    settings_save();storage_prepare_sleep();
    usb_prepare_sleep();
    board_prepare_sleep();
    /* Entering level-triggered sleep while BOOT is still low would wake again.
     * The screen is already off; wait for a debounced release before arming. */
    int released=0;
    while(released<4) {
        released=gpio_get_level(GPIO_NUM_0)?released+1:0;
        vTaskDelay(pdMS_TO_TICKS(10));
    }
    ESP_ERROR_CHECK(esp_sleep_disable_wakeup_source(ESP_SLEEP_WAKEUP_ALL));
    ESP_ERROR_CHECK(rtc_gpio_init(GPIO_NUM_0));
    ESP_ERROR_CHECK(rtc_gpio_set_direction(GPIO_NUM_0,RTC_GPIO_MODE_INPUT_ONLY));
    ESP_ERROR_CHECK(rtc_gpio_pullup_en(GPIO_NUM_0));
    ESP_ERROR_CHECK(rtc_gpio_pulldown_dis(GPIO_NUM_0));
    ESP_ERROR_CHECK(esp_sleep_enable_ext0_wakeup(GPIO_NUM_0,0));
    esp_deep_sleep_start();
}
void power_poll(void) {
    int64_t now=esp_timer_get_time();
    int event=power_button_update(&button,gpio_get_level(GPIO_NUM_0)==0,now);
    if(event==POWER_BUTTON_SHUTDOWN) power_off();
    int remaining=power_button_remaining(&button,now);
    if(remaining!=shown) {shown=remaining;ui_power_countdown(remaining);}
    if(event==POWER_BUTTON_RELEASE) ui_wake();
}
uint32_t power_hold_ms(void) {
    return button.armed&&button.pressed_at_us?(uint32_t)((esp_timer_get_time()-button.pressed_at_us)/1000):0;
}
int power_wake_reason(void) {return esp_sleep_get_wakeup_cause();}
