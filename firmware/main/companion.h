#pragma once
#include <stdbool.h>
#include <stdint.h>
#include "lvgl.h"
#define COMPANION_VERSION "1.0.11"
#define FRAME_MAX 8192
#define HISTORY_SIZE 1800
#define DRIVE_MAX 6
#define PROCESS_MAX 10
#define GPU_MAX 8
typedef struct {char name[80];float temp,load,power,vram,vram_total,hotspot,fan,fan_rpm,clock;int index;} gpu_t;
typedef struct { char name[48]; float used,total,temperature,health,read,write; } drive_t;
typedef struct { char name[36]; float cpu,ram,gpu; } process_t;
typedef struct {
    float cpu_load,cpu_temp,cpu_power,cpu_clock,cpu_voltage,thermal_temp;
    float gpu_load,gpu_temp,gpu_hotspot,gpu_power,gpu_clock,gpu_mem_clock,gpu_fan;
    float gpu_vram,gpu_vram_total,ram_used,ram_total,ram_available,ram_cached,ram_commit,swap_used;
    float net_down,net_up,net_ping,net_loss,net_utilization,ai_tps,ai_context,ai_vram,ai_cpu,ai_ram,ai_queue;
    char cpu_name[80],gpu_name[80],host_name[48],net_adapter[48],ai_model[72],ai_provider[24],ai_state[32],ai_note[64];
    char cpu_temp_source[96],thermal_name[64];
    gpu_t gpus[GPU_MAX];int gpu_count,gpu_total,gpu_offset,gpu_detected,gpu_load_available;
    drive_t drives[DRIVE_MAX]; process_t processes[PROCESS_MAX];
    float cores[32],core_temps[32],core_clocks[32];
    int drive_count,process_count,core_count;
    uint32_t seq; int64_t received_us; int64_t unix_time;
} telemetry_t;
typedef struct { uint8_t version,theme,brightness,fahrenheit,rate,animations,clock_mode; uint16_t timeout; uint8_t auto_rotate,center_mode; } settings_t;
extern settings_t settings;
typedef struct { const char *title; void (*build)(lv_obj_t *); void (*update)(const telemetry_t *,bool); } screen_t;
extern const screen_t screens[9];
void model_init(void);
bool model_accept(const char *data,size_t len);
void model_copy(telemetry_t *out);
void model_history(float *out,int metric,int seconds,int count);
void settings_load(void);
void settings_save(void);
void board_init(void);
void board_brightness(uint8_t value);
void board_prepare_sleep(void);
void power_init(void);
void power_poll(void);
uint32_t power_hold_ms(void);
int power_wake_reason(void);
bool board_imu(float *x,float *y,float *z);
uint8_t board_panel_id(void);
void board_auto_rotate(void);
int board_rotation(void);
void usb_init(void);
void usb_prepare_sleep(void);
void usb_send(const char *line);
void usb_action(const char *action);
void usb_settings_report(void);
void storage_init(void);
void storage_prepare_sleep(void);
uint32_t storage_flash_bytes(void);
bool storage_sd_ready(void);
uint64_t storage_sd_bytes(void);
const char *storage_sd_label(void);
void usb_capture(void);
void usb_poll(void);
void ui_init(void);
void ui_wake(void);
void ui_power_countdown(int remaining);
void ui_navigate(int index);
void ui_panel(int which);
void ui_refresh(void);
void ui_request_capture(void);
void ui_test_touch(int x,int y,int state);
bool ui_is_connected(void);
int ui_page(void);
uint32_t ui_theme_bg(void);
uint32_t ui_theme_fg(void);
uint32_t ui_accent(void);
lv_obj_t *ui_label(lv_obj_t *p,const char *txt,int x,int y,int w,const lv_font_t *font,uint32_t color);
lv_obj_t *ui_ring(lv_obj_t *p,int size,int y,int width,uint32_t color);
void ui_value(lv_obj_t *label,float value,const char *unit,int precision);
void ui_temp(lv_obj_t *label,float value);
void ui_animate_arc(lv_obj_t *arc,float value);
void overview_build(lv_obj_t *p); void overview_update(const telemetry_t *t,bool live);
int overview_mode(void);
void overview_select(int mode);
void overview_cycle(void);
void cpu_build(lv_obj_t *p); void cpu_update(const telemetry_t *t,bool live);
void gpu_build(lv_obj_t *p); void gpu_update(const telemetry_t *t,bool live);
void memory_build(lv_obj_t *p); void memory_update(const telemetry_t *t,bool live);
void storage_build(lv_obj_t *p); void storage_update(const telemetry_t *t,bool live);
void network_build(lv_obj_t *p); void network_update(const telemetry_t *t,bool live);
void process_build(lv_obj_t *p); void process_update(const telemetry_t *t,bool live);
void ai_build(lv_obj_t *p); void ai_update(const telemetry_t *t,bool live);
void settings_build(lv_obj_t *p); void settings_update(const telemetry_t *t,bool live);
bool storage_next(int delta);
