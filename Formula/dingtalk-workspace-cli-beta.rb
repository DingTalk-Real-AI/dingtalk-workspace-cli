class DingtalkWorkspaceCliBeta < Formula
  desc "Automate DingTalk workspace tasks from the terminal (beta channel)"
  homepage "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli"
  version "1.0.57-beta.3"
  license "Apache-2.0"
  keg_only "it is the beta channel and conflicts with dingtalk-workspace-cli"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.57-beta.3/dws-darwin-arm64.tar.gz"
      sha256 "b1ea300a76654751ea33540d8a244b0c81b5df3947786980f95a5a19362a097a"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.57-beta.3/dws-darwin-amd64.tar.gz"
      sha256 "68b5f6e38bec994fa4db4bef5db1629c7bce799bb9837c447008c3325fc886e3"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.57-beta.3/dws-linux-arm64.tar.gz"
      sha256 "350e74f1a2611975e476e113e50264975a98185c11ee889a83d9480c0f10181b"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.57-beta.3/dws-linux-amd64.tar.gz"
      sha256 "2c27a9a884650a7a60545d9447f1b966667216e94c333ebf9d69f9b2ead96e04"
    end
  end

  resource "skills" do
    url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.57-beta.3/dws-skills.zip"
    sha256 "abaa8feaa3c61fff048cfd1139e1fc5b91c329eb666d2eb852797a3f5c4c0cac"
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
