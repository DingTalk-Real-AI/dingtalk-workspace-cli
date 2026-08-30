class DingtalkWorkspaceCliBeta < Formula
  desc "Automate DingTalk workspace tasks from the terminal (beta channel)"
  homepage "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli"
  version "1.0.61-beta.2"
  license "Apache-2.0"
  keg_only "it is the beta channel and conflicts with dingtalk-workspace-cli"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.61-beta.2/dws-darwin-arm64.tar.gz"
      sha256 "066dfacc2bc73f80fd195ddffb3b5e380cdee4c164c0a8341aa592925c20f0c6"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.61-beta.2/dws-darwin-amd64.tar.gz"
      sha256 "184f1410e3de79f8bcffd619380ba47d743c9dce3dfab555620b827c48fdb6ab"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.61-beta.2/dws-linux-arm64.tar.gz"
      sha256 "c0008ae75e30b4f2e1ace11709e457c59dbf59601c1e465aabc0039f8094ed91"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.61-beta.2/dws-linux-amd64.tar.gz"
      sha256 "a8e47b750af8c33a3014de73d2bcdfbca5d769ee528533d676a7ac6c5c2f5f01"
    end
  end

  resource "skills" do
    url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.61-beta.2/dws-skills.zip"
    sha256 "9d59b854faf2c209de7631d55f3b356d7e4f7010ea70703c8eb04dc6f3d8f975"
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
