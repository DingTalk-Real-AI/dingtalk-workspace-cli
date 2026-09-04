class DingtalkWorkspaceCliBeta < Formula
  desc "Automate DingTalk workspace tasks from the terminal (beta channel)"
  homepage "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli"
  version "1.0.62-beta.4"
  license "Apache-2.0"
  keg_only "it is the beta channel and conflicts with dingtalk-workspace-cli"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.62-beta.4/dws-darwin-arm64.tar.gz"
      sha256 "63354c348f480fdd11d1e9e1422ae0e03923ff7d2328cef5eb5b5c0f23871d57"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.62-beta.4/dws-darwin-amd64.tar.gz"
      sha256 "af3d7f645d1008bd4f62cb2bf0f67b8176825444170feabe9cca8bee9122ee66"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.62-beta.4/dws-linux-arm64.tar.gz"
      sha256 "1b431c14f6627d33d8a73f67a4012ee1b9993ece18de436587b5ae7e148bfefc"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.62-beta.4/dws-linux-amd64.tar.gz"
      sha256 "53bae84fa8cc5c088b5df582de0b49159bd3e117a115b7cdab3cbc18fa8ccdc0"
    end
  end

  resource "skills" do
    url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.62-beta.4/dws-skills.zip"
    sha256 "22424d3ed3d5130550ca97a9d2dcfc9b93e8a4abbc0ff15b1b7b960d6e751458"
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
