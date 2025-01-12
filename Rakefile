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

desc "Build Docker image"
task :build_docker do
  system("docker build . -t mark24code/convert2ascii:latest")
end

desc "Push Docker image"
task :push_docker do
  system("docker push mark24code/convert2ascii:latest")
end

desc "Run in docker"
task :run_in_docker do
  system("docker run -it -v $(pwd):/app  mark24code/convert2ascii bash -c \"cd /app && exec bash\"")
end

task default: %i[]
