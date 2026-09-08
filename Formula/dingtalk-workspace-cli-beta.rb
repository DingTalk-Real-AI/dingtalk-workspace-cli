class DingtalkWorkspaceCliBeta < Formula
  desc "Automate DingTalk workspace tasks from the terminal (beta channel)"
  homepage "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli"
  version "1.0.62-beta.5"
  license "Apache-2.0"
  keg_only "it is the beta channel and conflicts with dingtalk-workspace-cli"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.62-beta.5/dws-darwin-arm64.tar.gz"
      sha256 "5a36f2cc473c74d69f940568564b7f76f7a03133a7353857063b896178984a10"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.62-beta.5/dws-darwin-amd64.tar.gz"
      sha256 "7ef49150673b95ac1a409712c3237e6ab1d6ba74f335c01a2a8b89d24e861f2b"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.62-beta.5/dws-linux-arm64.tar.gz"
      sha256 "1eaf5af6e2f8cb94b711d9fce3e8d8fe110e0ee508b6a01b55af857fec6e731c"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.62-beta.5/dws-linux-amd64.tar.gz"
      sha256 "ccbc90961c4e6bdb9fc15ad400cfbc76ebb231b652c60a2b04100ea4d02f8dbb"
    end
  end

  resource "skills" do
    url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.62-beta.5/dws-skills.zip"
    sha256 "aacf8d1a82700d6e01bcac5827ad61922ce7acd84857111d4a5add98ee6b4a0c"
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
