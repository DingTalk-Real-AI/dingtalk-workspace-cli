class DingtalkWorkspaceCli < Formula
  desc "Automate DingTalk workspace tasks from the terminal"
  homepage "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli"
  version "1.0.65"
  license "Apache-2.0"


  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.65/dws-darwin-arm64.tar.gz"
      sha256 "8550c9c42a2018204761db923911321a56df9bedcacf3d42baffae00ccd7bc44"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.65/dws-darwin-amd64.tar.gz"
      sha256 "6afdd8dcb093ea0c7aa9d0e76371f44308aa6ee9f8a44bf0d2b010bb2e3c8244"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.65/dws-linux-arm64.tar.gz"
      sha256 "8858dbaaa9552c4028aaa23c621d4ab960e1eda8d5078e238e7ec216ac2a00d7"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.65/dws-linux-amd64.tar.gz"
      sha256 "e712e01924935070ef0d313ef898bf90e5124c6c4ad9ef197c3647180b4be9cb"
    end
  end

  resource "skills" do
    url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.65/dws-skills.zip"
    sha256 "65d4cedcb4ba420932008205a422e886de100768e1ec51390d98916bb6689d19"
  end

  def install
    root = Dir["dws-*"].find { |entry| File.directory?(entry) } || "."
    binary = File.join(root, "dws")
    raise "binary not found: #{binary}" unless File.exist?(binary)

    libexec.install binary => "dws"
    bin.install_symlink libexec/"dws"

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
