#include "gpu_profiles.h"
#include <stdlib.h>
#include <string.h>
#include "nvs.h"
static cJSON *profiles;
bool gpu_profile_key(const char *key) {
    if(!key||strlen(key)!=32) return false;
    for(int i=0;i<32;i++) if(!((key[i]>='0'&&key[i]<='9')||(key[i]>='a'&&key[i]<='f'))) return false;
    return true;
}
static bool valid_hidden(const cJSON *hidden) {
    if(!cJSON_IsArray(hidden)||cJSON_GetArraySize(hidden)>16) return false;
    const cJSON *item;
    cJSON_ArrayForEach(item,hidden) if(!cJSON_IsString(item)||!gpu_profile_key(item->valuestring)) return false;
    return true;
}
void gpu_profiles_load(void) {
    profiles=cJSON_CreateObject();nvs_handle_t h;
    if(nvs_open("companion",NVS_READONLY,&h)!=ESP_OK) return;
    size_t size=0;
    if(nvs_get_str(h,"gpu_profiles",NULL,&size)==ESP_OK&&size>1&&size<=3800) {
        char *text=malloc(size);
        if(text&&nvs_get_str(h,"gpu_profiles",text,&size)==ESP_OK) {
            cJSON *saved=cJSON_Parse(text);bool valid=cJSON_IsObject(saved)&&cJSON_GetArraySize(saved)<=4;
            const cJSON *entry;
            cJSON_ArrayForEach(entry,saved) if(!gpu_profile_key(entry->string)||!valid_hidden(entry)) valid=false;
            if(valid) {cJSON_Delete(profiles);profiles=saved;} else cJSON_Delete(saved);
        }
        free(text);
    }
    nvs_close(h);
}
cJSON *gpu_profile_get(const char *host) {
    const cJSON *hidden=gpu_profile_key(host)?cJSON_GetObjectItemCaseSensitive(profiles,host):NULL;
    return hidden?cJSON_Duplicate(hidden,true):cJSON_CreateArray();
}
bool gpu_profile_set(const char *host,const cJSON *hidden) {
    if(!gpu_profile_key(host)||!valid_hidden(hidden)) return false;
    cJSON *next=cJSON_Duplicate(profiles,true);
    if(!next) return false;
    cJSON_DeleteItemFromObjectCaseSensitive(next,host);
    if(cJSON_GetArraySize(hidden)>0) {
        if(cJSON_GetArraySize(next)>=4) cJSON_DeleteItemFromObjectCaseSensitive(next,next->child->string);
        cJSON *copy=cJSON_Duplicate(hidden,true);
        if(!copy||!cJSON_AddItemToObject(next,host,copy)) {cJSON_Delete(copy);cJSON_Delete(next);return false;}
    }
    char *text=cJSON_PrintUnformatted(next);bool saved=false;nvs_handle_t h;
    if(text&&strlen(text)<3800&&nvs_open("companion",NVS_READWRITE,&h)==ESP_OK) {
        saved=nvs_set_str(h,"gpu_profiles",text)==ESP_OK&&nvs_commit(h)==ESP_OK;
        nvs_close(h);
    }
    free(text);
    if(saved) {cJSON_Delete(profiles);profiles=next;} else cJSON_Delete(next);
    return saved;
}
