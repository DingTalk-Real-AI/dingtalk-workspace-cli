class DingtalkWorkspaceCliBeta < Formula
  desc "Automate DingTalk workspace tasks from the terminal (beta channel)"
  homepage "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli"
  version "1.0.58-beta.3"
  license "Apache-2.0"
  keg_only "it is the beta channel and conflicts with dingtalk-workspace-cli"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.58-beta.3/dws-darwin-arm64.tar.gz"
      sha256 "29b4fb9e081f36a699933c0919fe7530544f62de3e27c785d27626a575a2efc2"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.58-beta.3/dws-darwin-amd64.tar.gz"
      sha256 "a6b9c4ef212c533e02b414bd9c3be0b8d1874d4af983a7896072825bdfec1033"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.58-beta.3/dws-linux-arm64.tar.gz"
      sha256 "ae3a9ebe151702fd05dee0c56d778a20789d98c390cf8c0a0b2ec695b57b5ec3"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.58-beta.3/dws-linux-amd64.tar.gz"
      sha256 "544b480701e9ec9ec5366467ad4c855fca06dea887e3d577ff50e4b3eeb13ac2"
    end
  end

  resource "skills" do
    url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.58-beta.3/dws-skills.zip"
    sha256 "322f1840442ff183ad4b6d4f2a2b38825ff9f96a3fcde06ba57b8f80647468ae"
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
