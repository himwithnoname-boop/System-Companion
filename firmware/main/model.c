#include "companion.h"
#include <math.h>
#include <stddef.h>
#include <string.h>
#include "cJSON.h"
#include "freertos/FreeRTOS.h"
#include "freertos/semphr.h"
#include "esp_timer.h"
static telemetry_t latest;
static SemaphoreHandle_t mutex;
static float history[4][HISTORY_SIZE];
static unsigned history_pos,history_count;
static int64_t history_sec;
static float number(cJSON *o,const char *key) {
    cJSON *v=cJSON_GetObjectItemCaseSensitive(o,key);
    return cJSON_IsNumber(v)&&isfinite(v->valuedouble)&&fabs(v->valuedouble)<1e12 ? (float)v->valuedouble:NAN;
}
static void string(cJSON *o,const char *key,char *dst,size_t cap) {
    cJSON *v=cJSON_GetObjectItemCaseSensitive(o,key);
    if(cJSON_IsString(v)) { size_t j=0; for(size_t i=0;v->valuestring[i]&&j<cap-1;i++) {
        unsigned char c=v->valuestring[i]; dst[j++]=(c>=32&&c!=127)?c:' '; } dst[j]=0; }
}
void model_init(void) {
    mutex=xSemaphoreCreateMutex(); assert(mutex);
    memset(&latest,0,sizeof(latest));
    float *f=&latest.cpu_load;
    for(size_t i=0;i<offsetof(telemetry_t,cpu_name)/sizeof(float);i++) f[i]=NAN;
    for(int a=0;a<4;a++) for(int b=0;b<HISTORY_SIZE;b++) history[a][b]=NAN;
}
bool model_accept(const char *data,size_t len) {
    if(len==0||len>=FRAME_MAX) return false;
    cJSON *o=cJSON_ParseWithLength(data,len+1); if(!o) return false;
    cJSON *kind=cJSON_GetObjectItemCaseSensitive(o,"type");
    cJSON *v=cJSON_GetObjectItemCaseSensitive(o,"v");
    if(!cJSON_IsObject(o)||!cJSON_IsString(kind)||strcmp(kind->valuestring,"telemetry")||!cJSON_IsNumber(v)||v->valueint!=1) {cJSON_Delete(o); return false;}
    telemetry_t t={0};
#define N(k) t.k=number(o,#k)
    N(thermal_temp);N(cpu_load);N(cpu_temp);N(cpu_power);N(cpu_clock);N(cpu_voltage);
    N(gpu_load);N(gpu_temp);N(gpu_hotspot);N(gpu_power);N(gpu_clock);N(gpu_mem_clock);N(gpu_fan);
    N(gpu_vram);N(gpu_vram_total);N(ram_used);N(ram_total);N(ram_available);N(ram_cached);N(ram_commit);N(swap_used);
    N(net_down);N(net_up);N(net_ping);N(net_loss);N(net_utilization);N(ai_tps);N(ai_context);N(ai_vram);N(ai_cpu);N(ai_ram);N(ai_queue);
#undef N
#define S(k) string(o,#k,t.k,sizeof(t.k))
    S(cpu_temp_source);S(thermal_name);S(cpu_name);S(gpu_name);S(host_name);S(net_adapter);S(ai_model);S(ai_provider);S(ai_state);S(ai_note);
#undef S
    cJSON *seq=cJSON_GetObjectItemCaseSensitive(o,"seq"); if(cJSON_IsNumber(seq)) t.seq=(uint32_t)seq->valuedouble;
    cJSON *clock=cJSON_GetObjectItemCaseSensitive(o,"time"); if(cJSON_IsNumber(clock)) t.unix_time=(int64_t)clock->valuedouble;
    cJSON *item,*a=cJSON_GetObjectItemCaseSensitive(o,"gpus");
    cJSON_ArrayForEach(item,a) {
        if(t.gpu_count>=GPU_MAX)break;
        if(!cJSON_IsObject(item))continue;
        gpu_t *g=&t.gpus[t.gpu_count++];string(item,"name",g->name,sizeof(g->name));
        cJSON *index=cJSON_GetObjectItemCaseSensitive(item,"index");g->index=cJSON_IsNumber(index)&&index->valueint>=0&&index->valueint<1024?index->valueint:t.gpu_count-1;
        g->temp=number(item,"temp");g->load=number(item,"load");g->power=number(item,"power");
        g->vram=number(item,"vram");g->vram_total=number(item,"vram_total");g->hotspot=number(item,"hotspot");
        g->fan=number(item,"fan");g->fan_rpm=number(item,"fan_rpm");g->clock=number(item,"clock");
    }
    t.gpu_total=t.gpu_count;t.gpu_offset=0;
    cJSON *gpu_total=cJSON_GetObjectItemCaseSensitive(o,"gpu_count"),*gpu_offset=cJSON_GetObjectItemCaseSensitive(o,"gpu_offset");
    if(cJSON_IsNumber(gpu_total)&&gpu_total->valueint>=t.gpu_count&&gpu_total->valueint<=1024)t.gpu_total=gpu_total->valueint;
    if(cJSON_IsNumber(gpu_offset)&&gpu_offset->valueint>=0&&gpu_offset->valueint<t.gpu_total)t.gpu_offset=gpu_offset->valueint;
    cJSON *detected=cJSON_GetObjectItemCaseSensitive(o,"gpu_detected"),*reporting=cJSON_GetObjectItemCaseSensitive(o,"gpu_load_available");
    t.gpu_detected=cJSON_IsNumber(detected)&&detected->valueint>=0&&detected->valueint<=1024?detected->valueint:t.gpu_total;
    t.gpu_load_available=cJSON_IsNumber(reporting)&&reporting->valueint>=0&&reporting->valueint<=1024?reporting->valueint:0;
    a=cJSON_GetObjectItemCaseSensitive(o,"drives");
    cJSON_ArrayForEach(item,a) { if(t.drive_count>=DRIVE_MAX) break; if(!cJSON_IsObject(item)) continue;
        drive_t *d=&t.drives[t.drive_count++];string(item,"name",d->name,sizeof(d->name));
        d->used=number(item,"used");d->total=number(item,"total");d->temperature=number(item,"temperature");
        d->health=number(item,"health");d->read=number(item,"read");d->write=number(item,"write"); }
    a=cJSON_GetObjectItemCaseSensitive(o,"processes");
    cJSON_ArrayForEach(item,a) { if(t.process_count>=PROCESS_MAX) break; if(!cJSON_IsObject(item)) continue;
        process_t *p=&t.processes[t.process_count++];string(item,"name",p->name,sizeof(p->name));
        p->cpu=number(item,"cpu");p->ram=number(item,"ram");p->gpu=number(item,"gpu"); }
    a=cJSON_GetObjectItemCaseSensitive(o,"cores");
    cJSON_ArrayForEach(item,a) { if(t.core_count>=32) break; int i=t.core_count++;
        t.cores[i]=number(item,"load");t.core_temps[i]=number(item,"temperature");t.core_clocks[i]=number(item,"clock"); }
    cJSON_Delete(o); t.received_us=esp_timer_get_time();
    xSemaphoreTake(mutex,portMAX_DELAY); latest=t;
    int64_t second=t.received_us/1000000;
    if(second!=history_sec) {
        int64_t missing=history_sec?second-history_sec-1:0;
        if(missing>HISTORY_SIZE) missing=HISTORY_SIZE;
        for(int64_t i=0;i<missing;i++) {
            for(int m=0;m<4;m++) history[m][history_pos]=NAN;
            history_pos=(history_pos+1)%HISTORY_SIZE;if(history_count<HISTORY_SIZE) history_count++;
        }
        history_sec=second; history[0][history_pos]=t.cpu_load;history[1][history_pos]=t.gpu_load;
        history[2][history_pos]=t.ram_total>0?t.ram_used/t.ram_total*100:NAN;history[3][history_pos]=t.net_down;
        history_pos=(history_pos+1)%HISTORY_SIZE;if(history_count<HISTORY_SIZE) history_count++; }
    xSemaphoreGive(mutex); return true;
}
void model_copy(telemetry_t *out) { xSemaphoreTake(mutex,portMAX_DELAY);*out=latest;xSemaphoreGive(mutex); }
void model_history(float *out,int metric,int seconds,int count) {
    xSemaphoreTake(mutex,portMAX_DELAY);
    for(int i=0;i<count;i++) {
        int back=(count-1-i)*seconds/count;out[i]=NAN;
        if(metric>=0&&metric<4&&back<(int)history_count) out[i]=history[metric][(history_pos+HISTORY_SIZE-1-back)%HISTORY_SIZE];
    } xSemaphoreGive(mutex);
}
