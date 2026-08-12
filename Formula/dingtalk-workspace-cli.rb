class DingtalkWorkspaceCli < Formula
  desc "Automate DingTalk workspace tasks from the terminal"
  homepage "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli"
  version "1.0.57"
  license "Apache-2.0"


  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.57/dws-darwin-arm64.tar.gz"
      sha256 "c01c28dc13948a70fca905207073dc8dbd22f7ba7fc90e68b3316eb9a9c98e88"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.57/dws-darwin-amd64.tar.gz"
      sha256 "d7baa218beefc851c6a933b456055195f8272984ce008d7e0122bdfc5dad94ea"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.57/dws-linux-arm64.tar.gz"
      sha256 "0bbe9c233a3ff585077bae1ac5000937c32d967846d14cc44c46f98d49b95ae2"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.57/dws-linux-amd64.tar.gz"
      sha256 "f113ce3654f21d1f9ecc7c196f815aeafbca54d377a347b244a15116c5cba698"
    end
  end

  resource "skills" do
    url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.57/dws-skills.zip"
    sha256 "0c9667209cf30761427a8f9348149cbbf1e397aa3c25587e99f205bc7525e101"
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
