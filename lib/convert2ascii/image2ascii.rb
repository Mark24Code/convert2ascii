require "io/console"
require "rmagick"
require "rainbow"
require "open-uri"
require_relative "./check_package"
require_relative './version'

module Convert2Ascii
  class Image2AsciiError < StandardError
  end

  class Image2Ascii
    module STYLE_ENUM
      Color = "color"
      Text = "text"
    end

    module COLOR_ENUM
      Full = "full"
      Greyscale = "greyscale"
    end


    attr_reader :width, :ascii_string
    attr_accessor :chars

    def initialize(**args)
      @uri = args[:uri]
      @width = args[:width] || IO.console.winsize[1]
      @style = args[:style] || STYLE_ENUM::Color # "color": color ansi , "text": plain text
      @color = args[:color] || COLOR_ENUM::Full # full
      @color_block = args[:color_block] || false

      check_quantum_convert_factor
      # divides quantum depth color space into usable rgb values
      @quantum_convert_factor = Magick::MAGICKCORE_QUANTUM_DEPTH == 16 ? 257 : 1

      @chars = ".'`^\",:;Il!i><~+_-?][}{1)(|\\/tfjrxnuvczXYUJCLQ0OZmwqpdbkhao*#MW&8%B@$"
      @ascii_string = ""
    end

    def check_quantum_convert_factor
      # quantum conversion factor for dealing with quantum depth color values
      if Magick::MAGICKCORE_QUANTUM_DEPTH > 16
        raise Image2AsciiError, "[Error] ImageMagick quantum depth is set to #{Magick::MAGICKCORE_QUANTUM_DEPTH}. It needs to be 16 or less"
      end
    end

    def generate(**args)
      @width = args[:width] || @width
      @style = args[:style] || @style # "color": color ansi , "text": plain text
      @color = args[:color] || @color # full
      @color_block = args[:color_block] || @color_block

      generate_string

      self
    end

    def tty_print
      print @ascii_string
    end

    private

    def check_packages
      CheckImageMagick.new.check
    end

    def generate_string
      resource = URI.open(@uri)
      img = Magick::ImageList.new
      img.from_blob(resource.read)

      img = correct_aspect_ratio(img, @width)

      img.each_pixel do |pixel, col, row|
        r, g, b, brightness = get_pixel_values(pixel)
        char = select_character(brightness)

        if @style == STYLE_ENUM::Text
          @ascii_string << char
        end

        if @style == STYLE_ENUM::Color
          chosen_color = get_chosen_color(@color, r, g, b)

          if @color_block == true
            @ascii_string << Rainbow(" ").background(*chosen_color)
          else
            @ascii_string << Rainbow(char).color(*chosen_color)
          end
        end

        # add line wrap once desired width is reached
        if (col % (@width - 1) == 0) and (col != 0)
          @ascii_string << "\n"
        end
      end
    end

    def correct_aspect_ratio(img, width)
      img = img.scale(width / img.columns.to_f)
      img = img.scale(img.columns, img.rows / 2)
    end

    def get_pixel_values(pixel)
      r = pixel.red / @quantum_convert_factor
      g = pixel.green / @quantum_convert_factor
      b = pixel.blue / @quantum_convert_factor
      # Brightness ref: https://en.wikipedia.org/wiki/Relative_luminance
      brightness = (0.2126 * r + 0.7152 * g + 0.0722 * b)

      return [r, g, b, brightness]
    end

    def select_character(brightness)
      char_index = brightness / (255.0 / @chars.length)
      @chars[char_index.floor]
    end

    def get_chosen_color(color, r, g, b)
      if color == COLOR_ENUM::Full
        [r, g, b]
      elsif color == COLOR_ENUM::Greyscale
        [r, r, r]
      else
        color
      end
    end
  end
end
