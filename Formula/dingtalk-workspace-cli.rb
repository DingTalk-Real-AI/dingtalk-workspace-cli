class DingtalkWorkspaceCli < Formula
  desc "Automate DingTalk workspace tasks from the terminal"
  homepage "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli"
  version "1.0.68"
  license "Apache-2.0"


  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.68/dws-darwin-arm64.tar.gz"
      sha256 "f517d980bdaf18bfc8b1ae92707419d8ddcbb876b4565bbf7ef235d7db79a0fb"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.68/dws-darwin-amd64.tar.gz"
      sha256 "92e49a50147d2295c129962db709ed906b88294d5975b963203eb5f72fd78c75"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.68/dws-linux-arm64.tar.gz"
      sha256 "bdcc2eb51d7c5d29f0dd88a527eab2d54bb3c09f042c721c0adfb5dd641f7556"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.68/dws-linux-amd64.tar.gz"
      sha256 "45be0f60c75a95d1287727ea0bfb8b2257a193106d9b008808813ce2ff167645"
    end
  end

  resource "skills" do
    url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.68/dws-skills.zip"
    sha256 "1ecde660aa0f2c907005878d07f3c5e8f29d450b0e563c4158b35facb7e1754c"
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
