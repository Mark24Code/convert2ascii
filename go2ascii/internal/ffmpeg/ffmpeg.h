#ifndef G2A_FFMPEG_H
#define G2A_FFMPEG_H

#include <libavformat/avformat.h>
#include <libavcodec/avcodec.h>
#include <libswscale/swscale.h>
#include <libswresample/swresample.h>
#include <stdint.h>

typedef struct {
    AVFormatContext *fmt;
    int video_stream;
    int audio_stream;
    AVCodecContext *video_ctx;
    AVCodecContext *audio_ctx;
    AVFrame *frame;
    AVFrame *aframe;
    AVPacket *pkt;
    struct SwsContext *sws;
    struct SwrContext *swr;
    uint8_t *audio_buf;
    int audio_buf_cap;
    int audio_buf_len;
    int first_pts_set;
    double first_pts;
    double time_base;
    int sample_rate;
    int nb_channels;

    // Segment decode / downscale state (zero by default; the legacy
    // DecodeFrames path never touches these, so it is unaffected).
    int window_enabled; // 1 after g2a_config; gates drop/skip logic
    double start_sec;   // t=0-relative abs window lower bound (seconds)
    double end_sec;     // abs upper bound; <= 0 means open window (to EOF)
    double abs_shift;   // stream start_time in seconds; subtract for t=0-relative
    int target_w;       // >0 => sws downscales to target on export
    int target_h;
} g2a;

int g2a_open(g2a *c, const char *path);
int g2a_config(g2a *c, double start_sec, double end_sec, int tw, int th);
int g2a_next_frame(g2a *c);
uint8_t *g2a_export_frame(g2a *c, int *w, int *h);
void g2a_frame_time(g2a *c, double *t);
void g2a_abs_frame_time(g2a *c, double *t);
int g2a_video_dims(g2a *c, int *w, int *h);
int g2a_audio_next(g2a *c);
void g2a_audio_take(g2a *c, uint8_t **buf, int *len);
double g2a_duration(g2a *c);
void g2a_free(void *p);
void g2a_close(g2a *c);

#endif
