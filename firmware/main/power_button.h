#pragma once
#include <stdbool.h>
#include <stdint.h>

typedef struct {
    int64_t raw_changed_us,pressed_at_us;
    bool raw_pressed,pressed,armed,fired;
} power_button_t;
enum { POWER_BUTTON_NONE,POWER_BUTTON_RELEASE,POWER_BUTTON_SHUTDOWN };

static inline int power_button_update(power_button_t *s,bool down,int64_t now) {
    if(s->fired) return POWER_BUTTON_NONE;
    if(down!=s->raw_pressed) {s->raw_pressed=down;s->raw_changed_us=now;}
    if(now-s->raw_changed_us<40000) return POWER_BUTTON_NONE;
    if(down!=s->pressed) {
        s->pressed=down;
        if(down&&s->armed) s->pressed_at_us=now;
        else if(!down) {
            bool released=s->armed&&s->pressed_at_us!=0;
            s->pressed_at_us=0;s->armed=true;
            return released?POWER_BUTTON_RELEASE:POWER_BUTTON_NONE;
        }
    }
    /* A held wake/boot press is ignored until a stable release. */
    if(!down) s->armed=true;
    if(s->armed&&s->pressed_at_us&&now-s->pressed_at_us>=10000000) {
        s->fired=true;return POWER_BUTTON_SHUTDOWN;
    }
    return POWER_BUTTON_NONE;
}
static inline int power_button_remaining(const power_button_t *s,int64_t now) {
    return s->armed&&s->pressed_at_us&&!s->fired?10-(int)((now-s->pressed_at_us)/1000000):-1;
}
