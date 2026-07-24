class DingtalkWorkspaceCliBeta < Formula
  desc "Automate DingTalk workspace tasks from the terminal (beta channel)"
  homepage "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli"
  version "1.0.55-beta.2"
  license "Apache-2.0"
  keg_only "it is the beta channel and conflicts with dingtalk-workspace-cli"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.55-beta.2/dws-darwin-arm64.tar.gz"
      sha256 "fd23353c473419ce8cfaf316e7afff757cc4606291f4ad41a62c6f5625f45c8f"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.55-beta.2/dws-darwin-amd64.tar.gz"
      sha256 "0b429950a6449736338b504386d8009a4f2cdb0668f64f72a61cd503a25df193"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.55-beta.2/dws-linux-arm64.tar.gz"
      sha256 "c213a22d6187a8f68596b9f25d6277f5fd58cea5120f5b7caeef011fb41f1104"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.55-beta.2/dws-linux-amd64.tar.gz"
      sha256 "4e041ba7b1d8038fa34deba3a3f78af8377f4361131d8bfdb1f1913e5f9454dc"
    end
  end

  resource "skills" do
    url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.55-beta.2/dws-skills.zip"
    sha256 "1cec445c73b9ff569ca002b6548f4ee8383712447bc4cebd7701c6065774524a"
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
