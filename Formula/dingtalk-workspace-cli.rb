class DingtalkWorkspaceCli < Formula
  desc "Automate DingTalk workspace tasks from the terminal"
  homepage "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli"
  version "1.0.60"
  license "Apache-2.0"


  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.60/dws-darwin-arm64.tar.gz"
      sha256 "4ed89f8a6f8341f83c78b48ce765fd18b1d7a305499aea78dfc16c6c5fecff68"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.60/dws-darwin-amd64.tar.gz"
      sha256 "53c569e2c713a8a2a0f73114068669e87d3a5fa78ec2bbeca5e19e49442adb37"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.60/dws-linux-arm64.tar.gz"
      sha256 "e6676617ff4f813907632a0848a34b45ea210776c0cc415fbe978f6ea626b2a6"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.60/dws-linux-amd64.tar.gz"
      sha256 "2d9158a2e67ef4eb193bb04a226d1360973c214cd781b3dab0099a214600f1a2"
    end
  end

  resource "skills" do
    url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.60/dws-skills.zip"
    sha256 "422b8e3353609850faf050007762598a438404156e65ad2585104944a3562591"
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
