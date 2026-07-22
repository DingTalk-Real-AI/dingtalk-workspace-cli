class DingtalkWorkspaceCli < Formula
  desc "Automate DingTalk workspace tasks from the terminal"
  homepage "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli"
  version "1.0.54"
  license "Apache-2.0"


  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.54/dws-darwin-arm64.tar.gz"
      sha256 "8ae0e52cf973f6fb3df61c67a41fd11e2df417a0c815762b6060cbcb5e600c08"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.54/dws-darwin-amd64.tar.gz"
      sha256 "11b711b9d70dea62304bf5f8206c56b4e7ea91148dafe97fb7c0f844a2a61da3"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.54/dws-linux-arm64.tar.gz"
      sha256 "9c7ecb4c8cd55644b2faa73f6ce7843c0279b23793e23deb5061692ea71a0cf1"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.54/dws-linux-amd64.tar.gz"
      sha256 "8a0bc245747fc3facf98c8103c06da46852a30bff31ac93b0aa874e8c7e46db7"
    end
  end

  resource "skills" do
    url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.54/dws-skills.zip"
    sha256 "7450fd0115c75bfe6820c7099f348973d9353cca9d8d647c9cddcd70978a7ec0"
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
