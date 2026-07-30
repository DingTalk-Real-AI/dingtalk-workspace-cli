class DingtalkWorkspaceCli < Formula
  desc "Automate DingTalk workspace tasks from the terminal"
  homepage "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli"
  version "1.0.55"
  license "Apache-2.0"


  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.55/dws-darwin-arm64.tar.gz"
      sha256 "dd753bbd051e5dd007cf433b8aa211c4a221dd73dfcb0b3783fa924d09f12351"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.55/dws-darwin-amd64.tar.gz"
      sha256 "f465eb7ac38a8a84eac4eb821fd15424bfc6f6245a60fa695ba97a639970dd77"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.55/dws-linux-arm64.tar.gz"
      sha256 "5961be0fd551ec8e69b6fff2b1609f73486f7e6c3ffe8eb4bb99fa1ed691b401"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.55/dws-linux-amd64.tar.gz"
      sha256 "051ba404a5f6a8fb15def0e0f5d9d273cf9d63f881df2fffe159f2c4ea3366e7"
    end
  end

  resource "skills" do
    url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.55/dws-skills.zip"
    sha256 "bd35f674f184001f5a03c7b5fa6029ebcda54f0054e15cd608b5b5e213ce2d05"
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
