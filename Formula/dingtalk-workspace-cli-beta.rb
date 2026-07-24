class DingtalkWorkspaceCliBeta < Formula
  desc "Automate DingTalk workspace tasks from the terminal (beta channel)"
  homepage "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli"
  version "1.0.55-beta.3"
  license "Apache-2.0"
  keg_only "it is the beta channel and conflicts with dingtalk-workspace-cli"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.55-beta.3/dws-darwin-arm64.tar.gz"
      sha256 "b833c5a1799b07c7c60a1f1c345b59422a4cda45d07fd6812ae0c29103b61777"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.55-beta.3/dws-darwin-amd64.tar.gz"
      sha256 "e5d49b6466c00d75de2589f3aa18e051c3f705621c05dba44f613a906dd8f198"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.55-beta.3/dws-linux-arm64.tar.gz"
      sha256 "bad99072427d4c1cf02028465db2b84bff61fabce95e3a04c56511575b02f2b7"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.55-beta.3/dws-linux-amd64.tar.gz"
      sha256 "564f0913d7e753d4de0edf6724d4b0e7541fe007fb573ed21dc819be88604534"
    end
  end

  resource "skills" do
    url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.55-beta.3/dws-skills.zip"
    sha256 "48283ee7d8c4f9a7f57a5a42f2aabe4c5fda89ee78cb750889e42347a2a671f9"
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
