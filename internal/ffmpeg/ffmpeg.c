#include "ffmpeg.h"
#include <stdlib.h>
#include <string.h>

int g2a_open(g2a *c, const char *path) {
    memset(c, 0, sizeof(*c));
    c->video_stream = -1;
    c->audio_stream = -1;
    if (avformat_open_input(&c->fmt, path, NULL, NULL) < 0) return -1;
    if (avformat_find_stream_info(c->fmt, NULL) < 0) return -2;
    for (unsigned i = 0; i < c->fmt->nb_streams; i++) {
        AVStream *st = c->fmt->streams[i];
        if (st->codecpar->codec_type == AVMEDIA_TYPE_VIDEO && c->video_stream < 0)
            c->video_stream = (int)i;
        if (st->codecpar->codec_type == AVMEDIA_TYPE_AUDIO && c->audio_stream < 0)
            c->audio_stream = (int)i;
    }
    if (c->video_stream >= 0) {
        AVStream *st = c->fmt->streams[c->video_stream];
        AVCodecParameters *par = st->codecpar;
        const AVCodec *codec = avcodec_find_decoder(par->codec_id);
        if (!codec) return -3;
        c->video_ctx = avcodec_alloc_context3(codec);
        if (!c->video_ctx) return -4;
        if (avcodec_parameters_to_context(c->video_ctx, par) < 0) return -5;
        if (avcodec_open2(c->video_ctx, codec, NULL) < 0) return -6;
        c->frame = av_frame_alloc();
        c->pkt = av_packet_alloc();
        c->time_base = av_q2d(st->time_base);
        // t=0 reference for absolute frame times (used by the parallel path).
        c->abs_shift = (st->start_time != AV_NOPTS_VALUE) ? st->start_time * c->time_base : 0.0;
        c->sws = sws_getContext(c->video_ctx->width, c->video_ctx->height,
                                c->video_ctx->pix_fmt,
                                c->video_ctx->width, c->video_ctx->height,
                                AV_PIX_FMT_RGBA, SWS_BILINEAR, NULL, NULL, NULL);
        if (!c->sws) return -20;
    }
    if (c->audio_stream >= 0) {
        AVStream *st = c->fmt->streams[c->audio_stream];
        AVCodecParameters *par = st->codecpar;
        const AVCodec *codec = avcodec_find_decoder(par->codec_id);
        if (!codec) return -7;
        c->audio_ctx = avcodec_alloc_context3(codec);
        if (!c->audio_ctx) return -8;
        if (avcodec_parameters_to_context(c->audio_ctx, par) < 0) return -9;
        if (avcodec_open2(c->audio_ctx, codec, NULL) < 0) return -10;
        c->aframe = av_frame_alloc();
        c->nb_channels = par->ch_layout.nb_channels;
        c->sample_rate = par->sample_rate;
        AVChannelLayout out_layout;
        av_channel_layout_default(&out_layout, c->nb_channels);
        if (swr_alloc_set_opts2(&c->swr, &out_layout, AV_SAMPLE_FMT_S16,
                                c->sample_rate, &par->ch_layout,
                                (enum AVSampleFormat)par->format, c->sample_rate,
                                0, NULL) < 0) return -11;
        if (swr_init(c->swr) < 0) return -12;
    }
    return 0;
}

// Absolute frame time (t=0-relative), falling back to first_pts when pts is
// unavailable. Used by the segment/parallel path and the window filter.
static double g2a_frame_abs_time(g2a *c) {
    if (!c->first_pts_set) {
        c->first_pts = (c->frame->pts != AV_NOPTS_VALUE)
                           ? c->frame->pts * c->time_base
                           : 0.0;
        c->first_pts_set = 1;
    }
    double frm = (c->frame->pts != AV_NOPTS_VALUE)
                     ? c->frame->pts * c->time_base
                     : c->first_pts;
    return frm - c->abs_shift;
}

// Returns 1 = keep frame, 0 = skip (pre-window), -1 = stop (segment end).
static int g2a_window_check(g2a *c) {
    if (!c->window_enabled) return 1;
    double t = g2a_frame_abs_time(c);
    if (t < c->start_sec) return 0;
    if (c->end_sec > 0 && t >= c->end_sec) return -1;
    return 1;
}

int g2a_config(g2a *c, double start_sec, double end_sec, int tw, int th) {
    if (c->video_stream < 0) return -1;
    if (tw > 0 && th > 0) {
        struct SwsContext *ns = sws_getContext(c->video_ctx->width,
                                c->video_ctx->height, c->video_ctx->pix_fmt,
                                tw, th, AV_PIX_FMT_RGBA,
                                SWS_FAST_BILINEAR, NULL, NULL, NULL);
        if (!ns) return -20;
        if (c->sws) sws_freeContext(c->sws);
        c->sws = ns;
        c->target_w = tw;
        c->target_h = th;
    }
    c->start_sec = start_sec;
    c->end_sec = end_sec;
    c->window_enabled = 1;
    if (start_sec > 0) {
        // Seek to the keyframe at/before start; frames before start_sec are
        // dropped by the window filter in g2a_next_frame.
        int64_t ts = (int64_t)((start_sec + c->abs_shift) / c->time_base);
        av_seek_frame(c->fmt, c->video_stream, ts, AVSEEK_FLAG_BACKWARD);
        avcodec_flush_buffers(c->video_ctx);
    }
    return 0;
}

int g2a_next_frame(g2a *c) {
    if (c->video_stream < 0) return 0;
    for (;;) {
        int ret = av_read_frame(c->fmt, c->pkt);
        if (ret < 0) {
            avcodec_send_packet(c->video_ctx, NULL);
            if (avcodec_receive_frame(c->video_ctx, c->frame) >= 0) {
                int wc = g2a_window_check(c);
                if (wc < 0) return 0;   // segment end reached
                if (wc == 0) continue;  // pre-window frame; flush path drains once
                return 1;
            }
            return 0;
        }
        if (c->pkt->stream_index == c->video_stream) {
            if (avcodec_send_packet(c->video_ctx, c->pkt) == 0) {
                if (avcodec_receive_frame(c->video_ctx, c->frame) == 0) {
                    int wc = g2a_window_check(c);
                    av_packet_unref(c->pkt);
                    if (wc < 0) return 0;
                    if (wc == 0) continue;
                    return 1;
                }
            }
        }
        av_packet_unref(c->pkt);
    }
}

void g2a_abs_frame_time(g2a *c, double *t) {
    *t = g2a_frame_abs_time(c);
}

int g2a_video_dims(g2a *c, int *w, int *h) {
    if (c->video_stream < 0 || !c->video_ctx) return -1;
    *w = c->video_ctx->width;
    *h = c->video_ctx->height;
    return 0;
}

void g2a_frame_time(g2a *c, double *t) {
    if (!c->first_pts_set) {
        c->first_pts = (c->frame->pts != AV_NOPTS_VALUE)
                           ? c->frame->pts * c->time_base
                           : 0.0;
        c->first_pts_set = 1;
    }
    double frm = (c->frame->pts != AV_NOPTS_VALUE)
                     ? c->frame->pts * c->time_base
                     : c->first_pts;
    *t = frm - c->first_pts;
}

uint8_t *g2a_export_frame(g2a *c, int *w, int *h) {
    // The sws context was built for video_ctx dims; if the decoded frame
    // differs, bail out rather than risk an undersized sws_scale destination.
    if (c->frame->width != c->video_ctx->width ||
        c->frame->height != c->video_ctx->height)
        return NULL;
    // When g2a_config set target dims, the sws context downscales on export.
    *w = (c->target_w > 0) ? c->target_w : c->frame->width;
    *h = (c->target_h > 0) ? c->target_h : c->frame->height;
    int size = (*w) * (*h) * 4;
    uint8_t *rgba = (uint8_t *)av_malloc((size_t)size);
    if (!rgba) return NULL;
    uint8_t *dst[4] = {rgba, NULL, NULL, NULL};
    int dst_stride[4] = {(*w) * 4, 0, 0, 0};
    sws_scale(c->sws, (const uint8_t *const *)c->frame->data,
              c->frame->linesize, 0, c->frame->height, dst, dst_stride);
    return rgba;
}

static int g2a_audio_convert(g2a *c) {
    uint8_t *out[1] = {NULL};
    int out_linesize = 0;
    int out_samples = swr_get_out_samples(c->swr, c->aframe->nb_samples) + 256;
    if (av_samples_alloc(out, &out_linesize, c->nb_channels, out_samples,
                         AV_SAMPLE_FMT_S16, 0) < 0)
        return -1;
    int converted = swr_convert(c->swr, out, out_samples,
                                (const uint8_t **)c->aframe->extended_data,
                                c->aframe->nb_samples);
    int bytes = converted * c->nb_channels * 2;
    if (bytes > 0) {
        if (c->audio_buf_len + bytes > c->audio_buf_cap) {
            int new_cap = (c->audio_buf_cap + bytes) * 2 + 1024;
            uint8_t *nb = (uint8_t *)realloc(c->audio_buf, (size_t)new_cap);
            if (!nb) { av_freep(&out[0]); return -2; }
            c->audio_buf = nb;
            c->audio_buf_cap = new_cap;
        }
        memcpy(c->audio_buf + c->audio_buf_len, out[0], (size_t)bytes);
        c->audio_buf_len += bytes;
    }
    av_freep(&out[0]);
    return bytes;
}

int g2a_audio_next(g2a *c) {
    if (c->audio_stream < 0) return 0;
    for (;;) {
        int ret = av_read_frame(c->fmt, c->pkt);
        if (ret < 0) {
            avcodec_send_packet(c->audio_ctx, NULL);
            while (avcodec_receive_frame(c->audio_ctx, c->aframe) == 0) {
                if (g2a_audio_convert(c) < 0) return -30;
            }
            return 0;
        }
        if (c->pkt->stream_index == c->audio_stream) {
            if (avcodec_send_packet(c->audio_ctx, c->pkt) == 0) {
                int produced = 0;
                while (avcodec_receive_frame(c->audio_ctx, c->aframe) == 0) {
                    if (g2a_audio_convert(c) < 0) { av_packet_unref(c->pkt); return -30; }
                    produced = 1;
                }
                av_packet_unref(c->pkt);
                if (produced) return 1;
                continue;
            }
        }
        av_packet_unref(c->pkt);
    }
}

void g2a_audio_take(g2a *c, uint8_t **buf, int *len) {
    *buf = c->audio_buf;
    *len = c->audio_buf_len;
    c->audio_buf = NULL;
    c->audio_buf_cap = 0;
    c->audio_buf_len = 0;
}

double g2a_duration(g2a *c) {
    if (c->fmt && c->fmt->duration != AV_NOPTS_VALUE)
        return (double)c->fmt->duration / AV_TIME_BASE;
    if (c->video_stream >= 0) {
        AVStream *st = c->fmt->streams[c->video_stream];
        if (st->duration != AV_NOPTS_VALUE) return st->duration * av_q2d(st->time_base);
    }
    return 0.0;
}

void g2a_free(void *p) {
    if (p) av_free(p);
}

void g2a_close(g2a *c) {
    if (c->sws) sws_freeContext(c->sws);
    if (c->swr) swr_free(&c->swr);
    if (c->frame) av_frame_free(&c->frame);
    if (c->aframe) av_frame_free(&c->aframe);
    if (c->pkt) av_packet_free(&c->pkt);
    if (c->video_ctx) avcodec_free_context(&c->video_ctx);
    if (c->audio_ctx) avcodec_free_context(&c->audio_ctx);
    if (c->fmt) avformat_close_input(&c->fmt);
    if (c->audio_buf) {
        free(c->audio_buf);
        c->audio_buf = NULL;
    }
}
