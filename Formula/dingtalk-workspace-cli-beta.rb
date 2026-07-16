class DingtalkWorkspaceCliBeta < Formula
  desc "Automate DingTalk workspace tasks from the terminal (beta channel)"
  homepage "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli"
  version "1.0.53-beta.2"
  license "Apache-2.0"
  keg_only "it is the beta channel and conflicts with dingtalk-workspace-cli"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.53-beta.2/dws-darwin-arm64.tar.gz"
      sha256 "08e4c0c5f40b720a823851073badcccda9c8ce9731f1f6f60bd2de3e9d3007ec"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.53-beta.2/dws-darwin-amd64.tar.gz"
      sha256 "306f301fabedf412c1326aa31343217d157df5c9485db32f8cbc8c33f592971a"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.53-beta.2/dws-linux-arm64.tar.gz"
      sha256 "6a6aaf9143ac63501f8104a76e4d8598a1330aab1ca287d31bcde73556ed8375"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.53-beta.2/dws-linux-amd64.tar.gz"
      sha256 "7b22bc0790651c69051205a3621046973ee978a65572e3e33a69a245ec92ac61"
    end
  end

  resource "skills" do
    url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.53-beta.2/dws-skills.zip"
    sha256 "cbfe8c3f8c60892b56a9c3f9090757f476112c7b556dccca2098148cdc387c1b"
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
