class DingtalkWorkspaceCliBeta < Formula
  desc "Automate DingTalk workspace tasks from the terminal (beta channel)"
  homepage "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli"
  version "1.0.69-beta.1"
  license "Apache-2.0"
  keg_only "it is the beta channel and conflicts with dingtalk-workspace-cli"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.69-beta.1/dws-darwin-arm64.tar.gz"
      sha256 "5c281a573b848c8c0354dd5d37743752d8d3caa4d71aba5d032293dc02f88aab"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.69-beta.1/dws-darwin-amd64.tar.gz"
      sha256 "5789f56175a4f7f12a64292adf27e90596821333bcc2aafb3f5475e4d1f9fb45"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.69-beta.1/dws-linux-arm64.tar.gz"
      sha256 "82a1c5ef9a39ef55b87d52055eae8ab88fa4abd0b9783a76e4ce450a6c967cbd"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.69-beta.1/dws-linux-amd64.tar.gz"
      sha256 "a6436dc8ff85193e9c7b6bcaf4bf6344def9ba32ee16fbdbdb6d8a60c71d3e93"
    end
  end

  resource "skills" do
    url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.69-beta.1/dws-skills.zip"
    sha256 "1cb70319e7a200604eca7ab57aa141b6af3a01d0b83e15a382011b1ff1bcf5e0"
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
