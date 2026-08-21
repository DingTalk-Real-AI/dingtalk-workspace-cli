class DingtalkWorkspaceCli < Formula
  desc "Automate DingTalk workspace tasks from the terminal"
  homepage "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli"
  version "1.0.59"
  license "Apache-2.0"


  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.59/dws-darwin-arm64.tar.gz"
      sha256 "61135a2a9286204ce060847e653c63c1e9784a0fa631bb7e0563b90628762a35"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.59/dws-darwin-amd64.tar.gz"
      sha256 "fd14b0b1a1475891fb243bf6453857a1044ab5a40bcf7dc1c7c795f57e5b03ba"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.59/dws-linux-arm64.tar.gz"
      sha256 "5bfe9ac7d1798b028f0fad579bbdffec5898e2fb16ee36f5766ab58e208abd50"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.59/dws-linux-amd64.tar.gz"
      sha256 "be1eb9a1f8fc5048e578b5b0bde212fc90baca0f289236c7c333d824bd869cf3"
    end
  end

  resource "skills" do
    url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.59/dws-skills.zip"
    sha256 "7ce5c3ab6f6a367407f64971bc5ff96cfcdfade2c1a10d326144b17c7b25a57e"
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

    EOS
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/dws version")
  end
end
