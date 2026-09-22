class DingtalkWorkspaceCliBeta < Formula
  desc "Automate DingTalk workspace tasks from the terminal (beta channel)"
  homepage "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli"
  version "1.0.63-beta.2"
  license "Apache-2.0"
  keg_only "it is the beta channel and conflicts with dingtalk-workspace-cli"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.63-beta.2/dws-darwin-arm64.tar.gz"
      sha256 "eb3be8cad5471f24c11b0380fd27ff7b0f286ae4451ff5f4552ef9d8b4f062a8"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.63-beta.2/dws-darwin-amd64.tar.gz"
      sha256 "7f42e38024a8d6bda808d4da1f20e32766b758b5e4e63c496ea97bfe3e963614"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.63-beta.2/dws-linux-arm64.tar.gz"
      sha256 "2046df3b8cc27ebdd22b7912ccb6e246168101d5780d55016045cadca1a446e5"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.63-beta.2/dws-linux-amd64.tar.gz"
      sha256 "be57af5baf4da7834b7f14aaedcd8f2f9596296ffe6e7ac30414e93d6d91a0d2"
    end
  end

  resource "skills" do
    url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.63-beta.2/dws-skills.zip"
    sha256 "ac0eb92b2f9ebf4b5bce25d37c47c2546e4c1e18c314dde38ece8c8937f2d5db"
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
