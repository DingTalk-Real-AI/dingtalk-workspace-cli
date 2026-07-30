class DingtalkWorkspaceCliBeta < Formula
  desc "Automate DingTalk workspace tasks from the terminal (beta channel)"
  homepage "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli"
  version "1.0.55-beta.8"
  license "Apache-2.0"
  keg_only "it is the beta channel and conflicts with dingtalk-workspace-cli"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.55-beta.8/dws-darwin-arm64.tar.gz"
      sha256 "07fabf720fa98f82c56027df703a3ad3f0aec16c957ea684047928f9f74fa00f"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.55-beta.8/dws-darwin-amd64.tar.gz"
      sha256 "fbec64dc5c3463a04de9720ffb1fd247196e619ea81b976b47ecbf751a17fb72"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.55-beta.8/dws-linux-arm64.tar.gz"
      sha256 "f111cdffef0188ddf954d2174fa0d318069bcec80104775483f13f645aa8089b"
    else
      url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.55-beta.8/dws-linux-amd64.tar.gz"
      sha256 "d69475b7f3cec4bad4c051834df22fbfadfa7075b82347e6ca7490f67d71d0b1"
    end
  end

  resource "skills" do
    url "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases/download/v1.0.55-beta.8/dws-skills.zip"
    sha256 "be8c9267704cfef1319fc9cd2fcba5aebe45b670b4dc37d3dae15f65523356f8"
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
