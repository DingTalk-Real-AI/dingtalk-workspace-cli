class DingtalkWorkspaceCliBeta < Formula
  desc "Automate DingTalk workspace tasks from the terminal (beta channel)"
  homepage "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli"
  version "1.0.65-beta.1"
  license "Apache-2.0"
  keg_only "it is the beta channel and conflicts with dingtalk-workspace-cli"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.65-beta.1/dws-darwin-arm64.tar.gz"
      sha256 "8b8d6e8f70b2d8e676a2b629317b11a84c86c8f166aa7a2fe8fc436671a7a4b6"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.65-beta.1/dws-darwin-amd64.tar.gz"
      sha256 "13e8e7e7933c5409df42c0f6cbafe48c58d0ed7c5715bb59e2fd5d351390abf0"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.65-beta.1/dws-linux-arm64.tar.gz"
      sha256 "771588717605a514915b9a065032e14dc3baa74e7b6002c9dc3e984e1680531c"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.65-beta.1/dws-linux-amd64.tar.gz"
      sha256 "e6e072d0bb45948bf5e840b57cbbe6ccf09f7768cb8926ee8b348c940746eac9"
    end
  end

  resource "skills" do
    url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.65-beta.1/dws-skills.zip"
    sha256 "55750ff745642bff3dead057ada0cb691a854f3bdac2dde95b894fa3a707195a"
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
