class DingtalkWorkspaceCli < Formula
  desc "Automate DingTalk workspace tasks from the terminal"
  homepage "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli"
  version "1.0.62"
  license "Apache-2.0"


  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.62/dws-darwin-arm64.tar.gz"
      sha256 "40dee66c299c0ce532b74f160e3c41c5f1dd938175dd2b7b8d4bcfa0d05fe3f7"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.62/dws-darwin-amd64.tar.gz"
      sha256 "70335ca7f4a5a535b3f3db2658ee7462c9d85cb18f037268aa7bcdf51d601af1"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.62/dws-linux-arm64.tar.gz"
      sha256 "7b015a3cf5104e4477786661fe42e97616961aa43ef6075bc5954b815d09a853"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.62/dws-linux-amd64.tar.gz"
      sha256 "6198a86570ea52f24d88a58dfe65514540793c4dd64410cb133a8ff7b3b008a8"
    end
  end

  resource "skills" do
    url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.62/dws-skills.zip"
    sha256 "0d347947118f67cee8bc84ec896777acfe8888db39d1ff5ee22d103dd5c433df"
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
