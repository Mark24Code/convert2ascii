FROM ruby:3.4.1-bookworm

RUN apt-get update
RUN apt-get install -y git cmake make gcc
RUN apt-get install -y ffmpeg libmagickwand-dev

RUN gem install convert2ascii


