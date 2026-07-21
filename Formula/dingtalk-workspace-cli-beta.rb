class DingtalkWorkspaceCliBeta < Formula
  desc "Automate DingTalk workspace tasks from the terminal (beta channel)"
  homepage "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli"
  version "1.0.53-beta.7"
  license "Apache-2.0"
  keg_only "it is the beta channel and conflicts with dingtalk-workspace-cli"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.53-beta.7/dws-darwin-arm64.tar.gz"
      sha256 "82bde5e8c777869dbba38e340e18346151be7147850e5701894c21daa4c0134e"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.53-beta.7/dws-darwin-amd64.tar.gz"
      sha256 "e09e59a7e483061c2c8755c2d87a94ee13c4f1ee8c4d4e14dfa61eff25c66ba1"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.53-beta.7/dws-linux-arm64.tar.gz"
      sha256 "2d80c06bc4e12304568c6e9719b465f0bbd2646f28317b38c46f74e94e272adf"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.53-beta.7/dws-linux-amd64.tar.gz"
      sha256 "2b624ecb5658903182ab01f5d2d17cd253fa2e2b8ce3e0516b5d6c2e4a567b0f"
    end
  end

  resource "skills" do
    url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.53-beta.7/dws-skills.zip"
    sha256 "75902633523a1bf988458a8717e2bce4cb6f954c10fb924a04d44f6f43310a62"
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
