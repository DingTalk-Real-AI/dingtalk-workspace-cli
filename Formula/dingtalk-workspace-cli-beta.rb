class DingtalkWorkspaceCliBeta < Formula
  desc "Automate DingTalk workspace tasks from the terminal (beta channel)"
  homepage "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli"
  version "1.0.54-beta.1"
  license "Apache-2.0"
  keg_only "it is the beta channel and conflicts with dingtalk-workspace-cli"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.54-beta.1/dws-darwin-arm64.tar.gz"
      sha256 "a667aaf453170b043df4d3e7c6c9f155e27446f7fd768c00fca6aed2a242816a"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.54-beta.1/dws-darwin-amd64.tar.gz"
      sha256 "8109ef196d2dce8477f54cd472b3c6ddbd0d95e51a8e605221c33236dc57c4c8"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.54-beta.1/dws-linux-arm64.tar.gz"
      sha256 "8dc636d858e0d7fd41b41036eb340845a43433dc3c614612da8e33a0c611fa57"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.54-beta.1/dws-linux-amd64.tar.gz"
      sha256 "4caf8055ba6d345a33c0e751bcba4b195e7494bf45b65a93cb80c755c1279e0f"
    end
  end

  resource "skills" do
    url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.54-beta.1/dws-skills.zip"
    sha256 "f4ddcaa6fb5846cfe9b80a5c49bcf3f66b00c624334e3df22d1b93c8f5117d45"
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
