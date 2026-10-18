// SPDX-License-Identifier: MIT
// Copyright (c) 2026 himwithnoname-boop
#include "read_lcd_id_bsp.h"
#include "lcd_probe.h"
#include "driver/gpio.h"
#include "esp_err.h"
#include "esp_rom_sys.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "esp_log.h"

static void set_level(int pin,int level) { gpio_set_level((gpio_num_t)pin,level); }
static void set_input(int enabled) { gpio_set_direction(PROBE_DATA,enabled?GPIO_MODE_INPUT:GPIO_MODE_OUTPUT); }
static int sample(void) { return gpio_get_level(PROBE_DATA); }
static void wait_us(uint32_t count) { esp_rom_delay_us(count); }
static void wait_ms(uint32_t count) { vTaskDelay(pdMS_TO_TICKS(count)); }

uint8_t read_lcd_id(void) {
    gpio_config_t pins={
        .pin_bit_mask=(1ULL<<9)|(1ULL<<10)|(1ULL<<11)|(1ULL<<12)|(1ULL<<13)|(1ULL<<14)|(1ULL<<21),
        .mode=GPIO_MODE_OUTPUT,.pull_up_en=GPIO_PULLUP_ENABLE,
        .pull_down_en=GPIO_PULLDOWN_DISABLE,.intr_type=GPIO_INTR_DISABLE,
    };
    ESP_ERROR_CHECK(gpio_config(&pins));
    const lcd_probe_bus_t bus={set_level,set_input,sample,wait_us,wait_ms};
    uint8_t id=lcd_probe_identify(&bus);
    ESP_LOGI("lcd_probe","controller ID: 0x%02x",id);
    return id;
}
