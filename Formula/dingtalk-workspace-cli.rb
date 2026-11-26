class DingtalkWorkspaceCli < Formula
  desc "Automate DingTalk workspace tasks from the terminal"
  homepage "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli"
  version "1.0.70"
  license "Apache-2.0"


  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.70/dws-darwin-arm64.tar.gz"
      sha256 "b3d09787741abd64c5a5372d6985ee0f451eaf87f6c6be0a6bfbc2bc9b37943e"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.70/dws-darwin-amd64.tar.gz"
      sha256 "72f85c67bfe0f491cf602495a9db275186b7bf353fd6fd3ae9b1fa5d6834fd93"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.70/dws-linux-arm64.tar.gz"
      sha256 "b5631700e7c86f88fdfcb9278aa28ca83577832991089a23cf77968e414d02b8"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.70/dws-linux-amd64.tar.gz"
      sha256 "b3273c91b23d7f300c5b1dae0a6737919fffc2badd3a9834de3289d3e89cdb81"
    end
  end

  resource "skills" do
    url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.70/dws-skills.zip"
    sha256 "b29cb5b7c532cddca314ea99f244174ef1f91c4c29936dceeafb12c3482904e1"
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
