class DingtalkWorkspaceCliBeta < Formula
  desc "Automate DingTalk workspace tasks from the terminal (beta channel)"
  homepage "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli"
  version "1.0.53-beta.5"
  license "Apache-2.0"
  keg_only "it is the beta channel and conflicts with dingtalk-workspace-cli"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.53-beta.5/dws-darwin-arm64.tar.gz"
      sha256 "2d97f31bbb59f26eeddeb8f1869d639c26407e5b3f2ca5d742a40fb03326d4f0"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.53-beta.5/dws-darwin-amd64.tar.gz"
      sha256 "92a6a1ede223743f6262b3d3f7968f314f9bb26e104e7e8cb943f9d9e0297d06"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.53-beta.5/dws-linux-arm64.tar.gz"
      sha256 "8083a101469c81681f1aaf295aba6c30bf5a69ecd17939dcec33be4399cf57e9"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.53-beta.5/dws-linux-amd64.tar.gz"
      sha256 "70559027b6e2f40973331cd7ef229329a9005271cb06db2ffa215c2714101361"
    end
  end

  resource "skills" do
    url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.53-beta.5/dws-skills.zip"
    sha256 "87eefe2f2a89bc57c3e7fa93ef92cc494dcca673307b9135e734ab21d28f834f"
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
