class DingtalkWorkspaceCliBeta < Formula
  desc "Automate DingTalk workspace tasks from the terminal (beta channel)"
  homepage "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli"
  version "1.0.60-beta.3"
  license "Apache-2.0"
  keg_only "it is the beta channel and conflicts with dingtalk-workspace-cli"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.60-beta.3/dws-darwin-arm64.tar.gz"
      sha256 "93aac820fcb3fa62e41ae5b4d8136c59e6f59ed98bfa7ed48b9cbb9f0ad22077"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.60-beta.3/dws-darwin-amd64.tar.gz"
      sha256 "474e1c9b6e7579479eb2f9d785cb2b8df8bd28cecc635c212d98a45cd4ae0dda"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.60-beta.3/dws-linux-arm64.tar.gz"
      sha256 "fc8366c3c83e5e1d9e58e00ebb2776e5a9dd034aa4c5f1b655dd6ce11329e8ff"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.60-beta.3/dws-linux-amd64.tar.gz"
      sha256 "5976bbb41ae1bf6186bcef21f1a5029b3ae96a0e10c51806905c185063b9173f"
    end
  end

  resource "skills" do
    url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.60-beta.3/dws-skills.zip"
    sha256 "ea34a72b72f2cf384278712abd3bd7a05a318fc1e42783c70955ad1c44376fe1"
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
