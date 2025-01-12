require 'rainbow'

module Convert2Ascii
  module OS

    module OS_ENUM
      MacOS = :macos
      Linux = :linux
      Windows = :ms
      Unknow = :unknow
    end

    def detect_os
      if RUBY_PLATFORM =~ /linux/
        return OS_ENUM::Linux
      elsif RUBY_PLATFORM =~ /darwin/
        return OS_ENUM::MacOS
      elsif RUBY_PLATFORM =~ /mswin|mingw|cygwin/
        return OS_ENUM::Windows
      else
        return OS_ENUM::Unknow
      end
    end
  end

  class CheckPackageError < StandardError
  end

  class CheckPackage
    include OS

    def initialize
      @os_name = nil
    end

    def macos_check
    end

    def ms_check
    end

    def linux_check
    end

    def unknow_check
      raise CheckPackageError, Rainbow("[Error] #{@os_name} not support!").red
    end


    def macos_installed?(package_name)
      output = `brew list #{package_name} 2>/dev/null`
      return output != ""
    end

    def detect_linux_distribution

      if !File.exist?('/etc/os-release')
        raise CheckPackageError, Rainbow("[Error] can not detect_os! #{@os_name} ").red
      end

      os_release = File.read('/etc/os-release')

      if os_release.include?('debian')
        # debian/ubuntu
        return'debian'
      end

      if os_release.include?('centos')
        # centos
        return'centos'
      end

      if os_release.include?('redhat')
        # redhat
        return'redhat'
      end

      if os_release.include?('arch')
        # arch linux
        return'arch'
      end

      raise CheckPackageError, Rainbow("[Error] #{@os_name} linux platform not support!").red
    end


    def debian_installed?(package_name)
      output = `dpkg -l | grep #{package_name}`
      !output.strip.empty?
    end

    def centos_installed?(package_name)
      output = `rpm -qa | grep #{package_name}`
      !output.strip.empty?
    end

    def redhat_installed?(package_name)
      output = `yum list installed | grep #{package_name}`
      !output.strip.empty?
    end

    def arch_installed?(package_name)
      output = `pacman -Q | grep #{package_name}`
      !output.strip.empty?
    end

    def check
      @os_name = detect_os
      __send__ "#{@os_name}_check"
    end
  end

  class CheckFFmpeg < CheckPackage
    def initialize
      super
      @name = "ffmpeg"
      @need_error = Rainbow("[Error] `#{@name}` is need!").red
      @tips = Rainbow("[Tips ] For more details and install: https://www.ffmpeg.org/").green
    end

    def macos_check
      if !macos_installed?(@name)
        raise CheckPackageError, "\n#{@need_error}\n#{@tips}\n"
      end
    end

    def linux_check
      linux_platform_name = detect_linux_distribution
      __send__ "#{linux_platform_name}_installed?", @name
    end

  end

  class CheckImageMagick < CheckPackage
    def initialize
      super
      @name = "imagemagick"
      @need_error = Rainbow("[Error] `imagemagick` is need!").red
      @tips = Rainbow("[Tips ] For more details and install guide: https://github.com/rmagick/rmagick").green

    end

    def macos_check
      if !macos_installed?(@name)
        raise CheckPackageError, "\n#{@need_error}\n#{@tips}\n"
      end
    end

    def linux_check
      # just Debian/Ubuntu
      linux_pkg = {
        "debian" => "libmagickwand-dev",
        "redhat" => "ImageMagick-devel",
        "arch" => "imagemagick",
      }

      linux_platform_name = detect_linux_distribution
      pkg_name = linux_pkg.fetch(linux_platform_name, nil)

      if !pkg_name
        raise CheckPackageError, "\n#{@need_error}\n#{@tips}\n"
      end

      __send__ "#{linux_platform_name}_installed?", pkg_name
    end
  end
end
