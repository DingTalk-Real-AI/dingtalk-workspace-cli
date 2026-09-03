class DingtalkWorkspaceCliBeta < Formula
  desc "Automate DingTalk workspace tasks from the terminal (beta channel)"
  homepage "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli"
  version "1.0.62-beta.2"
  license "Apache-2.0"
  keg_only "it is the beta channel and conflicts with dingtalk-workspace-cli"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.62-beta.2/dws-darwin-arm64.tar.gz"
      sha256 "56d99217c553a214004fac9f7541cb89ff74e66db8b49bea78e2b08fdc0b4b05"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.62-beta.2/dws-darwin-amd64.tar.gz"
      sha256 "d9fd3a415a3562d2ba855fe4cf3dd9ffc3b8370e2ae54d77cfff2be81d638e06"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.62-beta.2/dws-linux-arm64.tar.gz"
      sha256 "942b2c3f763976c042dd4bb966abb64895788a35cda5b8a45029f4a0b0f2637e"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.62-beta.2/dws-linux-amd64.tar.gz"
      sha256 "d3d3ce3225cbb1dd1c0a8b6ec5bd2c8ad3b7e3e70e7535cca46fd4e2092cd377"
    end
  end

  resource "skills" do
    url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.62-beta.2/dws-skills.zip"
    sha256 "4eca831417acd66c01a50ee84a079b4a0ca6a55667db075f8324bfd9e6fb1936"
  end

  def install
    root = Dir["dws-*"].find { |entry| File.directory?(entry) } || "."
    binary = File.join(root, "dws")
    raise "binary not found: #{binary}" unless File.exist?(binary)

    libexec.install binary => "dws"
    bin.install_symlink libexec/"dws"

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
