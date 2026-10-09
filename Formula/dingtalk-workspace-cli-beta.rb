class DingtalkWorkspaceCliBeta < Formula
  desc "Automate DingTalk workspace tasks from the terminal (beta channel)"
  homepage "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli"
  version "1.0.63-beta.3"
  license "Apache-2.0"
  keg_only "it is the beta channel and conflicts with dingtalk-workspace-cli"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.63-beta.3/dws-darwin-arm64.tar.gz"
      sha256 "d43ac4ce6a2eaebcc7eee8fcf2f928cbc597974be92c624e0edc0eebc6369d72"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.63-beta.3/dws-darwin-amd64.tar.gz"
      sha256 "616777c35e47b08f0186b7ada7109f83820037f09c0b92a7f3c733c3d7ef3fbd"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.63-beta.3/dws-linux-arm64.tar.gz"
      sha256 "a6bb24efd9221ce1f6f8b962ea92e6fd80d99d123dd460c333aa6dda26d6d616"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.63-beta.3/dws-linux-amd64.tar.gz"
      sha256 "b31420d6874e3668eb3c33e37dfd07e326417cc335f0a127823c619956955332"
    end
  end

  resource "skills" do
    url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.63-beta.3/dws-skills.zip"
    sha256 "ac0eb92b2f9ebf4b5bce25d37c47c2546e4c1e18c314dde38ece8c8937f2d5db"
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
      This beta is keg-only. Add #{opt_bin} to PATH to use its `dws` binary.
    EOS
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/dws version")
  end
end
