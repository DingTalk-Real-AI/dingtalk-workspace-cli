class DingtalkWorkspaceCliBeta < Formula
  desc "Automate DingTalk workspace tasks from the terminal (beta channel)"
  homepage "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli"
  version "1.0.59-beta.2"
  license "Apache-2.0"
  keg_only "it is the beta channel and conflicts with dingtalk-workspace-cli"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.59-beta.2/dws-darwin-arm64.tar.gz"
      sha256 "7f11218d3222f3e93c3b1e94b3a004c061eb0a297b206447ea95fe6b2b1ec674"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.59-beta.2/dws-darwin-amd64.tar.gz"
      sha256 "72f06a334cf29d23123639fabe13bf2951765066136e47ccc6453667c382f8c2"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.59-beta.2/dws-linux-arm64.tar.gz"
      sha256 "3edbfabb7718b53914a2d9efe7b1152e9ebd70765ff0b6f19ad3125e4a2458ed"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.59-beta.2/dws-linux-amd64.tar.gz"
      sha256 "e1c610070a9c1b3763656cbd53c818290f199097adc07cb9fe629ed765c5e1e0"
    end
  end

  resource "skills" do
    url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.59-beta.2/dws-skills.zip"
    sha256 "aa2854651eaa2c857b526aaec73fd1358d31b11999fe7fd3e99e5060b2522a1f"
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
