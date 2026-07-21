class DingtalkWorkspaceCli < Formula
  desc "Automate DingTalk workspace tasks from the terminal"
  homepage "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli"
  version "1.0.53"
  license "Apache-2.0"


  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.53/dws-darwin-arm64.tar.gz"
      sha256 "504df1b720f0114b69b31bf57ed8e25c7342f28fc8f8a977f7e59e2e724a49e6"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.53/dws-darwin-amd64.tar.gz"
      sha256 "979937a184c067db67f1e5d104a97de3bfdb6e203169e3a872455c48d819ed70"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.53/dws-linux-arm64.tar.gz"
      sha256 "a171684af5d6d4b7d53f8a4e0bb2ec90ad92b5872195558504dc60c6eb7db759"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.53/dws-linux-amd64.tar.gz"
      sha256 "6523ca7e11d08f7e402d98dd78f85d3c9fed88b123d69fa6d6e49fad9660c08c"
    end
  end

  resource "skills" do
    url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.53/dws-skills.zip"
    sha256 "9c5deef3fdbf8d07a054e74c96f6498ff714e574002bc115f3271a7eb5af3e90"
  end

  def install
    root = Dir["dws-*"].find { |entry| File.directory?(entry) } || "."
    binary = File.join(root, "dws")
    raise "binary not found: #{binary}" unless File.exist?(binary)

    bin.install binary => "dws"

    %w[LICENSE NOTICE README.md CHANGELOG.md].each do |name|
      source = File.join(root, name)
      pkgshare.install source if File.exist?(source)
    end

    skill_dest = pkgshare/"skills/dws"
    skill_dest.mkpath
    resource("skills").stage do
      cp_r(Dir["*"], skill_dest)
    end
  end

  def caveats
    <<~EOS
      Agent Skills are bundled in #{pkgshare}/skills/dws.
      Run `dws skill setup` to install them into your Agent directories.

    EOS
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/dws version")
  end
end
