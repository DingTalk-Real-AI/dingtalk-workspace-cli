class DingtalkWorkspaceCliBeta < Formula
  desc "Automate DingTalk workspace tasks from the terminal (beta channel)"
  homepage "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli"
  version "1.0.54-beta.2"
  license "Apache-2.0"
  keg_only "it is the beta channel and conflicts with dingtalk-workspace-cli"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.54-beta.2/dws-darwin-arm64.tar.gz"
      sha256 "46b57bed1f6e9f7ba007d8a86a6f5eb280fdeb557fc9bb5946f14f9b1f8f0c9f"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.54-beta.2/dws-darwin-amd64.tar.gz"
      sha256 "1b7fd08e64b1c86bbcee217604ffe07e0e8f1b3b5c4de518534386972bcf0f9b"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.54-beta.2/dws-linux-arm64.tar.gz"
      sha256 "108d3861ef606519f9934530d29654eab55a73607d1ee6775461f98ef5a6acd4"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.54-beta.2/dws-linux-amd64.tar.gz"
      sha256 "6cb96ee09419bbbcc1eb336218ac2aa1d9ca0ed5cbd5a80c79bc20e1e1f03ff7"
    end
  end

  resource "skills" do
    url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.54-beta.2/dws-skills.zip"
    sha256 "572b93f04a10268d185ad1f8e70e0d412949ae056be8494a9949387076fd14bc"
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
