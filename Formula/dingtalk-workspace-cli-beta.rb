class DingtalkWorkspaceCliBeta < Formula
  desc "Automate DingTalk workspace tasks from the terminal (beta channel)"
  homepage "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli"
  version "1.0.58-beta.1"
  license "Apache-2.0"
  keg_only "it is the beta channel and conflicts with dingtalk-workspace-cli"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.58-beta.1/dws-darwin-arm64.tar.gz"
      sha256 "34f7dabf1a97ed950c1cd331e5fb873a62c87632db249ca1b42bd2da925927f0"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.58-beta.1/dws-darwin-amd64.tar.gz"
      sha256 "c812ad070ac76f8ab5aefe2ae1b9cdf9217389b383296acb36070fc2f40b9890"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.58-beta.1/dws-linux-arm64.tar.gz"
      sha256 "7dd5e38647c88e4b7936e0f41315cd612128faeb7960f50b28dbbd1fa9683d68"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.58-beta.1/dws-linux-amd64.tar.gz"
      sha256 "b384671440a62f1e5dca5dcdb49e51ef1aad816b601021ba363c8c69986ebd2f"
    end
  end

  resource "skills" do
    url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.58-beta.1/dws-skills.zip"
    sha256 "2cf96aa2d35a01a4b92db2bd2d1760476d62d7528de5ec50a997c9a7713c3513"
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
