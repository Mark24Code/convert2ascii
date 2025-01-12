require "rainbow"
require_relative "./terminal"

module Convert2Ascii
  class TerminalPlayer

    SAFE_SLOW_DELTA = 0.9 # seconds
    SAFE_FAST_DELTA = 0.2 # seconds

    attr_accessor :play_loop, :step_duration, :debug
    def initialize(**args)
      @debug = false
      @audio = args[:audio]
      @frames = args[:frames]
      @play_loop = args[:play_loop]
      @step_duration = args[:step_duration]

      @total_duration = @frames.length * @step_duration
      @first_frame = true
      @backspace_adjust = "\033[A" * (@frames.length + 1)

      regist_hook
    end

    def play
      begin
        init_screen
        render
      rescue => error
        raise error
      ensure
        clean_up
      end
    end

    private

    def regist_hook
      trap("INT") {
        clean_up
        exit 0
      }

      # TODO  ??? 改变会消失
      # # 窗口变化事件
      # trap("SIGWINCH") {
      #   resize
      # }
    end

    def resize
      clear_screen
    end

    def clear_screen
      Terminal.clear_buffer
    end

    def init_screen
      clear_screen
      setup
    end

    def setup
      Terminal.open_buffer
      Terminal.hide_cursor
      Terminal.clear_screen
    end

    def clean_up
      if !@debug
        Terminal.close_buffer
        Terminal.clear_screen
        Terminal.show_cursor
      end
    end

    def debug_log(var_name)
      puts Rainbow('-- debug ----').yellow
      puts "class:"
      p var_name.class
      puts "value:"
      p var_name
    end

    def full_screen(content)

      if !content
        return content
      end
      # 补齐高度，没内容也要追加 \n 刷新，因为会拖拽 窗体尺寸会变化
      rows, _ = Terminal.winsize
      # content: pure text with "\n"
      content = content.split("\n")

      rows_fill = []
      delta = rows - content.length

      if delta > 0
        delta.times do
          rows_fill << "\n"
        end
      elsif delta < 0
        content = content[0..(rows - 1)]
      end

      content.join("\n")
    end

    def self_adaption_frame_play
      # 当偏移在-90ms（音频滞后于视频）到+20ms（音频超前视频）之间时，人感觉不到视听质量的变化，这个区域可以认为是同步区域
      # https://github.com/0voice/audio_video_streaming/blob/main/article/029-%E9%9F%B3%E8%A7%86%E9%A2%91%E5%90%8C%E6%AD%A5%E7%AE%97%E6%B3%95.md

      start_time = Time.now
      if @audio
        Thread.new do
          start_time = Time.now # 以音频为准
          play_cmd = "ffplay -nodisp "
          if @play_loop
            play_cmd << " -loop 0"
          end
          system("#{play_cmd} -i #{@audio} &> /dev/null")
          if @debug
            puts Rainbow("[info] audio time: #{Time.now - start_time} s").green
          end
        end
      end
      frame_index = 0
      loop do
        if frame_index <= @frames.length
          content = @frames[frame_index]
          if !@first_frame
            $stdout.print @backspace_adjust
          end
          clear_screen
          $stdout.print(full_screen(content))
          @first_frame = false
          sleep(@step_duration)
        end

        actual_time = Time.now - start_time
        video_play_time = frame_index * @step_duration

        offset_time = actual_time - video_play_time
        if offset_time > SAFE_SLOW_DELTA
          offset = (offset_time / @step_duration).floor
          if @debug
            puts Rainbow("[+] #{offset}").green
          end
        elsif offset_time < -1 * SAFE_FAST_DELTA
          offset = 0
        else
          offset = 1
        end

        frame_index = (frame_index + offset) % @frames.length

        real_time = Time.now - start_time
        frame_time = (frame_index + 1) * @step_duration

        if !@play_loop
          if real_time > @total_duration
            break
          end
        end

        if @debug
          puts ""
          puts Rainbow("RealTime: #{real_time} s").green
          puts Rainbow("FrameTime: #{frame_time} s").green
          puts Rainbow("CurrentFrame: #{frame_index} / #{@frames.length} ").green
          puts Rainbow("Real - Frame: #{real_time - frame_time} s").green
          puts Rainbow("[+] #{offset}").green
          puts Rainbow("@step_duration #{@step_duration}").green
          puts Rainbow("@play_loop #{@play_loop}").green
        end
      end
    end

    def render
      self_adaption_frame_play
    end
  end
end
