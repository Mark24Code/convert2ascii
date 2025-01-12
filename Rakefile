# frozen_string_literal: true
require "bundler/gem_tasks"

desc "Run Test"
task :test do
  tests = Dir.glob("./test/test_*.rb")
  tests.each do |t|
    system("ruby #{t}")
  end
end

desc "Build Rdoc"
task :build_rdoc do
  system("rdoc build")
end



task default: %i[]
