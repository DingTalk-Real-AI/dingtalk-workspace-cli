class DingtalkWorkspaceCli < Formula
  desc "Automate DingTalk workspace tasks from the terminal"
  homepage "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli"
  version "1.0.69"
  license "Apache-2.0"


  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.69/dws-darwin-arm64.tar.gz"
      sha256 "b7dbea8653bcdaa78934f7e632ed8cb7835ba1647b58ddc9b09c6981fc5560d8"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.69/dws-darwin-amd64.tar.gz"
      sha256 "4ab7511b7481531195a0d2d6cb16df9546e9513f3d375a165fd1afc892b0e559"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.69/dws-linux-arm64.tar.gz"
      sha256 "4a8fdb369527abbb4bcf90ebf79c5e329a2021c2608491f71a610ef3a67a9797"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.69/dws-linux-amd64.tar.gz"
      sha256 "567257be28c1ccce4f424fc92969f9c21dd15ed600a86c252adbc35821bf7c4a"
    end
  end

  resource "skills" do
    url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.69/dws-skills.zip"
    sha256 "45911a6d51b4449638a6fb773e6d00493c47f4b4178c839da592b21644ffe70b"
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

    EOS
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/dws version")
  end
end
