class DingtalkWorkspaceCli < Formula
  desc "Automate DingTalk workspace tasks from the terminal"
  homepage "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli"
  version "1.0.58"
  license "Apache-2.0"


  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.58/dws-darwin-arm64.tar.gz"
      sha256 "7d98599f90cae9d42b51ff2863efc87dbfb4a3176ff3c84fc2216110c0157a70"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.58/dws-darwin-amd64.tar.gz"
      sha256 "4c12e35e5bf7e0905812cd42dc94a5345068a2c16e306bb50b13c5c78b5cb95d"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.58/dws-linux-arm64.tar.gz"
      sha256 "5ef6bde24bc3db6a11a0f1d0b3343a048956b2cbcf6cd3409a037fb6ba425489"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.58/dws-linux-amd64.tar.gz"
      sha256 "3ccadcc6f070a39d2b2ba20429a4fcdc2f21639bf79f34361dc7d16f501bfda6"
    end
  end

  resource "skills" do
    url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.58/dws-skills.zip"
    sha256 "2626debc21c3daadfd155b4c167b2219b97e801398fe4441a8b48138960ab264"
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
