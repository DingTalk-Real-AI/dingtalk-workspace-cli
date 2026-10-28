class DingtalkWorkspaceCli < Formula
  desc "Automate DingTalk workspace tasks from the terminal"
  homepage "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli"
  version "1.0.66"
  license "Apache-2.0"


  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.66/dws-darwin-arm64.tar.gz"
      sha256 "5dabdf00c849c512c65150853a9c8c2d677ab9d809242d4b6304b2433049e7cb"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.66/dws-darwin-amd64.tar.gz"
      sha256 "313b64b136b3e9d41cbfaaa25677f1ba79d51223527e870a9ac6d34f0747d741"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.66/dws-linux-arm64.tar.gz"
      sha256 "4d6bd477d5b47d2595baa9adf718e3d38f6d2532614873e4f670cd84adf88096"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.66/dws-linux-amd64.tar.gz"
      sha256 "27cd2821e1f6523d1eeaf5688c23502bddbdd858cf82e32c197853065418bef8"
    end
  end

  resource "skills" do
    url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.66/dws-skills.zip"
    sha256 "30a872a3b472af235baf633662d26ad1ddd0c47d75c31b4452b2ea82dd966e44"
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
