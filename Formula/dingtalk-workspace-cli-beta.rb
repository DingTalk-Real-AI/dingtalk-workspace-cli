class DingtalkWorkspaceCliBeta < Formula
  desc "Automate DingTalk workspace tasks from the terminal (beta channel)"
  homepage "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli"
  version "1.0.60-beta.1"
  license "Apache-2.0"
  keg_only "it is the beta channel and conflicts with dingtalk-workspace-cli"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.60-beta.1/dws-darwin-arm64.tar.gz"
      sha256 "8ef11c79b5c86ec275dd82334232e7582f9e2ba99a66307d7681e42e8f53767b"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.60-beta.1/dws-darwin-amd64.tar.gz"
      sha256 "67612f1dac735984b026c7f8a0dc057beec4cdd029f0a97798bf90aa923eb2d3"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.60-beta.1/dws-linux-arm64.tar.gz"
      sha256 "67a8d4f4e0a7d22a9cc53cb91d8c97ecd1152665ce669f68560d86cec5987dd2"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.60-beta.1/dws-linux-amd64.tar.gz"
      sha256 "a5fae548b495842779df4291cbcf06d8a2e5ddddf68a41cad1bab1e5c64a1d59"
    end
  end

  resource "skills" do
    url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.60-beta.1/dws-skills.zip"
    sha256 "9fe12683139a626d32a801dd44158a698f142b61339282e0fc24d4e3a5e97e87"
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
