class DingtalkWorkspaceCliBeta < Formula
  desc "Automate DingTalk workspace tasks from the terminal (beta channel)"
  homepage "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli"
  version "1.0.63-beta.1"
  license "Apache-2.0"
  keg_only "it is the beta channel and conflicts with dingtalk-workspace-cli"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.63-beta.1/dws-darwin-arm64.tar.gz"
      sha256 "a9264feddc5400af38c84690a5a25da432fe3eeec816171916a36fbd5f71f4f0"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.63-beta.1/dws-darwin-amd64.tar.gz"
      sha256 "d4523f230658bf765757faee93b6739c6193b55b008f4adc57f0c5f0766f7b76"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.63-beta.1/dws-linux-arm64.tar.gz"
      sha256 "80fe9297af75c68aef8767a0950226c0c87206c03b4609991ed542d1e8826746"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.63-beta.1/dws-linux-amd64.tar.gz"
      sha256 "aa3d2d390c8a3f58072e841c471b17bfef87551950074694a7c7b932a9c40853"
    end
  end

  resource "skills" do
    url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.63-beta.1/dws-skills.zip"
    sha256 "83bc2317df514f3fb754e183fbd015d35b52e7b55767458c0a2e8d67bda790ef"
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
