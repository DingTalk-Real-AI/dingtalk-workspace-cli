class DingtalkWorkspaceCliBeta < Formula
  desc "Automate DingTalk workspace tasks from the terminal (beta channel)"
  homepage "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli"
  version "1.0.57-beta.2"
  license "Apache-2.0"
  keg_only "it is the beta channel and conflicts with dingtalk-workspace-cli"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.57-beta.2/dws-darwin-arm64.tar.gz"
      sha256 "2119754d4c6f6be2b4856ab559ad44ac582a3b3abc76ff907927f62c7a4a3d29"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.57-beta.2/dws-darwin-amd64.tar.gz"
      sha256 "a453341d6df1a78b7d74bd624842503d857a41f73fa1ac36394e4594e4961e8d"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.57-beta.2/dws-linux-arm64.tar.gz"
      sha256 "734df2c7f34ca36aa48151fda2b18e1c2c90fe812fb5ab13e8c00e074cca43af"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.57-beta.2/dws-linux-amd64.tar.gz"
      sha256 "f602a63ab6afd2e24db7b7dabfddb0cdcf3a7bd55b0cc60a99013bac5cacc56f"
    end
  end

  resource "skills" do
    url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.57-beta.2/dws-skills.zip"
    sha256 "486f5ef30a88a293c14df1ff0768760284179993c51f898fa2bee2c9391d8607"
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
