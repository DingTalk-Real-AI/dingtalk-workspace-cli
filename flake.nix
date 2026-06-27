{
  description = "DingTalk Workspace CLI development environment";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = { self, nixpkgs, ... }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f system);
    in
    {
      packages = forAllSystems (system:
        let
          pkgs = import nixpkgs { inherit system; };
        in
        rec {
          dws = pkgs.buildGoModule {
            pname = "dingtalk-workspace-cli";
            version = "dev";
            src = ./.;

            subPackages = [ "cmd" ];
            vendorHash = "sha256-8pt8NE5JhY+Ewl528PGaCxTEH4N0rPZudPE8rfhiVrk=";

            ldflags = [
              "-s"
              "-w"
            ];

            env.CGO_ENABLED = "0";

            postInstall = ''
              if [ -e "$out/bin/cmd" ] && [ ! -e "$out/bin/dws" ]; then
                mv "$out/bin/cmd" "$out/bin/dws"
              fi
            '';

            meta = with pkgs.lib; {
              description = "DingTalk Workspace CLI";
              homepage = "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli";
              license = licenses.asl20;
              mainProgram = "dws";
              platforms = platforms.unix;
            };
          };

          default = dws;
        });

      apps = forAllSystems (system: {
        dws = {
          type = "app";
          program = "${self.packages.${system}.dws}/bin/dws";
        };
        default = self.apps.${system}.dws;
      });

      devShells = forAllSystems (system:
        let
          pkgs = import nixpkgs { inherit system; };
        in
        {
          default = pkgs.mkShell {
            packages = with pkgs; [
              go_1_25
              gopls
              delve
              gotools
              direnv
              git
            ];

            env.CGO_ENABLED = "0";

            shellHook = ''
              echo "Entered dingtalk-workspace-cli dev shell (Go $(go version | awk '{ print $3 }'))"
            '';
          };
        });
    };
}
