#pragma once
#include <stdbool.h>
#include "cJSON.h"
void gpu_profiles_load(void);
bool gpu_profile_key(const char *key);
cJSON *gpu_profile_get(const char *host);
bool gpu_profile_set(const char *host,const cJSON *hidden);
