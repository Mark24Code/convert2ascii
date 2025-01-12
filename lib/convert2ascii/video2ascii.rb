require "open-uri"
require "json"
require "tmpdir"
require "etc"
require "io/console"
require "rainbow"
require_relative "./image2ascii"
require_relative "./terminal-player"
require_relative "./multi-tasker"
require_relative "./check_package"
require_relative './version'

module Convert2Ascii
  class Video2AsciiError < StandardError
  end

  class Video2Ascii
    DEFAULT_STEP_DURATION = 0.04


    attr_accessor :uri, :width, :threads_count, :output, :step_duration
    def initialize(**args)
      @uri = args[:uri]
      @step_duration = args[:step_duration] || DEFAULT_STEP_DURATION
      @threads_count = set_threads_count

      @tmpdir = nil
      @output = Dir.pwd

      # image2ascii attrs
      @width = args[:width] || IO.console.winsize[1]
      @style = args[:style] || Image2Ascii::STYLE_ENUM::Color # "color": color ansi , "text": plain text
      @color = args[:color] || Image2Ascii::COLOR_ENUM::Full # full
      @color_block = args[:color_block] || false

      check_packages
      regist_hooks
    end

    def generate(**args)
      @width = args[:width] || @width
      @style = args[:style] || @style # "color": color ansi , "text": plain text
      @color = args[:color] || @color # full
      @color_block = args[:color_block] || @color_block

      @tmpdir = Dir.mktmpdir
      @audio = get_audio_from_video(@tmpdir)
      screenshots_from_video(@tmpdir)
      convert_all_images(@tmpdir)
      @frames_path = order_frames_path

      self
    end


    def save(output_dir)
      system("rm -rf #{@tmpdir}/*.jpg")
      system("rm -rf #{output_dir} && mkdir #{output_dir}")
      system("cp -r #{@tmpdir}/* #{output_dir}")

      # save config
      File.open("#{output_dir}/meta.json", "w") do |f|
        config = {
          step_duration: @step_duration,
          audio: @audio ?  File.basename(@audio) : nil,
          frames_count: @frames_path.length
        }
        json_data = JSON.generate(config, pretty: true)
        f.puts json_data
      end

      puts ""
      puts Rainbow("[info] save success!").green
    end

    def order_frames_path
      frames_path = Dir.glob("#{@tmpdir}/*.txt")
      frames_path = frames_path.sort do |a, b|
        get_name_order(a) <=> get_name_order(b)
      end

      @frames_path =  frames_path
    end

    def play(**args)
      play_loop = args[:play_loop] || false
      step_duration = args[:step_duration] || @step_duration
      frames = @frames_path.map { |f| File.open(f).read }

      player_args = {
        frames: ,
        audio: @audio,
        play_loop:,
        step_duration:,
      }
      TerminalPlayer.new(**player_args).play

      return true
    end

    private

    def check_packages
      CheckImageMagick.new.check
      CheckFFmpeg.new.check
    end

    def regist_hooks
      at_exit {
        if @tmpdir
          FileUtils.remove_entry @tmpdir
        end
      }
    end

    def set_threads_count
      cpu_threads = (Etc.nprocessors || 1)
      cpu_threads = cpu_threads > 4 ? cpu_threads - 1 : cpu_threads
      cpu_threads
    end

    def get_audio_from_video(save_dir)
      puts Rainbow("[info] parsing audio...").green
      audio_save = "#{save_dir}/audio.mp3"
      cmd = "ffmpeg -i '#{@uri}' -vn  -nostats -loglevel 0   #{audio_save}"
      result = system(cmd)
      if !result
        audio_save = nil
      end
      puts Rainbow("[info] parsing audio done.").green

      return audio_save
    end

    def screenshots_from_video(save_dir)
      # ffmpeg use multi threads
      # Benchmark
      # Apple M1 Max:
      # * thread:1   2.64s
      # * thread:9   0.96s
      puts Rainbow("[info] video slicing...").green
      image_path = save_dir
      cmd = "ffmpeg -i '#{@uri}' -threads #{@threads_count} -vf fps=1/#{@step_duration}  -nostats -loglevel 0   #{image_path}/%d.jpg"
      result = system(cmd)

      if !result
        raise Video2AsciiError, Rainbow("\n[Error] exec `#{cmd}` fail!").red
      end
      puts Rainbow("[info] video slicing done.").green

      return image_path
    end

    def convert_to_ascii(image)
      config = {
        width: @width,
        style: @style,
        color: @color,
        color_block: @color_block
      }
      Image2Ascii.new(uri: image).generate(**config).ascii_string
    end

    def get_name_order(name_path)
      if name_path
        return File.basename(name_path, ".*").to_i
      end
    end

    def convert_all_images(save_dir)
      images = Dir.glob("#{save_dir}/*.jpg")

      processed_count = 0
      tasks = []
      time_start = Time.now
      images.each_with_index do |image, i|
        tasks << lambda {
          begin
            ascii_string = convert_to_ascii(image)
            basename = File.basename(image, ".jpg")
            File.open("#{@tmpdir}/#{basename}.txt", "w") do |f|
              f.puts ascii_string
            end

            processed_count += 1
            print(Rainbow("\rprocessing...  #{sprintf("%.2f", (1.0 * processed_count / images.length) * 100)} % (time: #{sprintf("%.2f", Time.now - time_start)} s)").green)
          rescue => error
            puts "-------"
            puts Rainbow("\n[Error] convert_to_ascii -> image: #{image} | i: #{i}").red
            puts error
          end
        }
      end

      MultiTasker.new(tasks).run
    end

  end
end
