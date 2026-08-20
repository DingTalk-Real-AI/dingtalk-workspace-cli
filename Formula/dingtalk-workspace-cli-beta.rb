class DingtalkWorkspaceCliBeta < Formula
  desc "Automate DingTalk workspace tasks from the terminal (beta channel)"
  homepage "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli"
  version "1.0.59-beta.4"
  license "Apache-2.0"
  keg_only "it is the beta channel and conflicts with dingtalk-workspace-cli"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.59-beta.4/dws-darwin-arm64.tar.gz"
      sha256 "f788467e9979c70ef210b411ac915b1506ea77ffa496e26b53cfa99650158721"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.59-beta.4/dws-darwin-amd64.tar.gz"
      sha256 "a01988709c0dc99dd5874859eb265ba08a6fda412a7ead8303c68e61d2a8b195"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.59-beta.4/dws-linux-arm64.tar.gz"
      sha256 "8e1a993b2137a082a8cc1d9535dfc2d7b3e4399c76d295840f9dc1f15cca7a0d"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.59-beta.4/dws-linux-amd64.tar.gz"
      sha256 "26e4cd72cfb96b38ef808863391b81a5c45c3170bca56b5eac457fc601b000c5"
    end
  end

  resource "skills" do
    url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.59-beta.4/dws-skills.zip"
    sha256 "a75107bdc14b5476e097842acc92f798301d8ffb59de9ade01f863d166a89435"
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
