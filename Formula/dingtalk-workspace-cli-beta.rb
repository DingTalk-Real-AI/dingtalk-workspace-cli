class DingtalkWorkspaceCliBeta < Formula
  desc "Automate DingTalk workspace tasks from the terminal (beta channel)"
  homepage "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli"
  version "1.0.58-beta.2"
  license "Apache-2.0"
  keg_only "it is the beta channel and conflicts with dingtalk-workspace-cli"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.58-beta.2/dws-darwin-arm64.tar.gz"
      sha256 "1b2b6953f7f1ae1ca6ecb0702424ac0e1a976a6a5ff91e8ffc3b5ae495d98c7c"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.58-beta.2/dws-darwin-amd64.tar.gz"
      sha256 "a1c1b3c58b48e04c0ae520062f9d6ab0dc961eddb635497bdb9b4345316e45f6"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.58-beta.2/dws-linux-arm64.tar.gz"
      sha256 "7f35e3c4734f17b125a8c32f3c95e05d1410f683cf6956be857ee9349f8e4d36"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.58-beta.2/dws-linux-amd64.tar.gz"
      sha256 "37beb9e39790563cf0584ac23376f713bf2eb2c50cff4222965e831ac9adbb0e"
    end
  end

  resource "skills" do
    url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.58-beta.2/dws-skills.zip"
    sha256 "7e10fead4192059c98d596c5b1886f77fd550526de5cd18c425cdad6fd64cd3a"
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
      This beta is keg-only. Add #{opt_bin} to PATH to use its `dws` binary.
    EOS
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/dws version")
  end
end
