class DingtalkWorkspaceCliBeta < Formula
  desc "DingTalk Workspace CLI"
  homepage "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli"
  version "1.0.52-beta.4"
  license "Apache-2.0"
  keg_only "it is the beta channel and conflicts with dingtalk-workspace-cli"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.52-beta.4/dws-darwin-arm64.tar.gz"
      sha256 "264691a806411d9c040aea824a219132ad48188bb6a23bb5a117363e1b712fb6"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.52-beta.4/dws-darwin-amd64.tar.gz"
      sha256 "2e52ba67ab148d6a09f5728ba344faee305556eec0605e6425df0b85a60a1a4e"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.52-beta.4/dws-linux-arm64.tar.gz"
      sha256 "b37e3674f5b4bbc5915afcb114d6ef682a570906a0cc2fd47ecc5648bd953a7d"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.52-beta.4/dws-linux-amd64.tar.gz"
      sha256 "742a06ac86a6eaf06836807be9ed0f0c007302368db6c40f9b65c81bc52772f8"
    end
  end

  resource "skills" do
    url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.52-beta.4/dws-skills.zip"
    sha256 "cbfa3aadda5dd7734a9562e3a43af193bbec6d679f37b69c6edab8f7cb81bf0b"
  end

  def install
    require "fileutils"

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
      FileUtils.cp_r(Dir["*"], skill_dest)
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
