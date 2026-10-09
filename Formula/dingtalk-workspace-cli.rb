class DingtalkWorkspaceCli < Formula
  desc "Automate DingTalk workspace tasks from the terminal"
  homepage "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli"
  version "1.0.63"
  license "Apache-2.0"


  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.63/dws-darwin-arm64.tar.gz"
      sha256 "7f57c3e4e141b9f0fd81a04f0298023a0855e1b6e02ad09c897e91e9c58bd5a9"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.63/dws-darwin-amd64.tar.gz"
      sha256 "87added1a0b2b2192516283b210ae90ab3770f55518560243c3c1905824485ed"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.63/dws-linux-arm64.tar.gz"
      sha256 "14df04191b8d3826ca48a26be290eae65bd3fcc15e45fbb580ae1d1542b891fc"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.63/dws-linux-amd64.tar.gz"
      sha256 "78cab668bf671c17128ac55347627fb28f80c15b9c30c5f86e32e2b5ee6e34bc"
    end
  end

  resource "skills" do
    url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.63/dws-skills.zip"
    sha256 "6a36e5a501fd8eb748702b1dedb33f93a166c0a88b459b6f982d926bfb8e150b"
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
