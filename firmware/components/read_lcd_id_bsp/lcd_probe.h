// SPDX-License-Identifier: MIT
// Copyright (c) 2026 himwithnoname-boop
#pragma once
#include <stdint.h>

// Board wiring and the 0x03/0xDA register-read transaction are hardware facts.
// The callbacks keep the protocol sequence independently testable.
enum { PROBE_CS=9, PROBE_CLOCK=10, PROBE_DATA=11, PROBE_RESET=21 };
typedef struct {
    void (*level)(int pin,int value);
    void (*input)(int enabled);
    int (*sample)(void);
    void (*microseconds)(uint32_t count);
    void (*milliseconds)(uint32_t count);
} lcd_probe_bus_t;

static inline uint8_t lcd_probe_identify(const lcd_probe_bus_t *bus) {
    bus->level(PROBE_CS,0);
    bus->level(PROBE_RESET,1);bus->milliseconds(120);
    bus->level(PROBE_RESET,0);bus->milliseconds(120);
    bus->level(PROBE_RESET,1);bus->milliseconds(120);
    const uint32_t request=0x0300da00u;
    for(int bit=31;bit>=0;--bit) {
        bus->level(PROBE_DATA,(request>>bit)&1u);
        bus->level(PROBE_CLOCK,0);bus->level(PROBE_CLOCK,1);
    }
    uint8_t result=0;
    for(int bit=0;bit<8;++bit) {
        bus->level(PROBE_CLOCK,0);
        bus->input(1);bus->microseconds(1);
        result=(uint8_t)((result<<1)|(bus->sample()&1));
        bus->input(0);bus->level(PROBE_CLOCK,1);bus->microseconds(1);
    }
    return result;
}
