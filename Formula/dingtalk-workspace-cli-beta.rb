class DingtalkWorkspaceCliBeta < Formula
  desc "Automate DingTalk workspace tasks from the terminal (beta channel)"
  homepage "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli"
  version "1.0.55-beta.1"
  license "Apache-2.0"
  keg_only "it is the beta channel and conflicts with dingtalk-workspace-cli"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.55-beta.1/dws-darwin-arm64.tar.gz"
      sha256 "7bb716eeb50c2fbc7f325af07e5749e77951d1459359e5362a47194a0931fb3f"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.55-beta.1/dws-darwin-amd64.tar.gz"
      sha256 "ec5590e2ebc172faf26a4f3c91db3c2dbe80b0926e0704dfc55a0ab5457d423f"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.55-beta.1/dws-linux-arm64.tar.gz"
      sha256 "b71651b431241a871dbe39ae3ad9cebcf3b66a699f48304dcab9e5cdfab698ec"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.55-beta.1/dws-linux-amd64.tar.gz"
      sha256 "fe95a3769cb793f28ce487fc1bb275678d50e6bc2fe1c1a712960489638a6570"
    end
  end

  resource "skills" do
    url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.55-beta.1/dws-skills.zip"
    sha256 "c3b141d0608af8f221c9efbe43d44cabd470e0895bf974f89fd4614a020e8608"
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
