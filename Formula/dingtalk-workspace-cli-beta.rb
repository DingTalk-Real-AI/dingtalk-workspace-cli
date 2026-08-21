class DingtalkWorkspaceCliBeta < Formula
  desc "Automate DingTalk workspace tasks from the terminal (beta channel)"
  homepage "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli"
  version "1.0.59-beta.5"
  license "Apache-2.0"
  keg_only "it is the beta channel and conflicts with dingtalk-workspace-cli"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.59-beta.5/dws-darwin-arm64.tar.gz"
      sha256 "274d56599a8e33ccca86a139424cab95a54ba311d6b643bccb2d3e6608cd16b4"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.59-beta.5/dws-darwin-amd64.tar.gz"
      sha256 "0a0a00a77ca24c102204cd6b7de3de58f406a7e0fad2dd965a4c1fe903c34f39"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.59-beta.5/dws-linux-arm64.tar.gz"
      sha256 "1e7af6393979c2fa433af9207722989749f11ea8e09ff9bcd5696e505d6d7f88"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.59-beta.5/dws-linux-amd64.tar.gz"
      sha256 "4896e71a1417acc3d8834fa99f0e81511e020ff952e57c0562cc255e90acec80"
    end
  end

  resource "skills" do
    url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.59-beta.5/dws-skills.zip"
    sha256 "11000b9c3566e3b38e3037b6b3069d55c8f50725b3ed9cd67714a7ebb794cd47"
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
