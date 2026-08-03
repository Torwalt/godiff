{
  description = "godiff — interactive terminal navigator for git diffs";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs =
    { self, nixpkgs }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
    in
    {
      packages = forAllSystems (pkgs: rec {
        default = godiff;
        godiff = pkgs.buildGoModule {
          pname = "godiff";
          version = "0.1.0";
          src = self;
          vendorHash = "sha256-WlKtVO/wmbc6a8+BRJUS3vSKy8l/GX/jTUuy8V3jUyg=";
          # unit and integration tests run in checkPhase and shell out to git
          nativeCheckInputs = [ pkgs.git ];
          preCheck = "export HOME=$TMPDIR";
          ldflags = [
            "-s"
            "-w"
            "-X main.version=0.1.0"
          ];
          meta = {
            description = "Interactive terminal navigator for git diffs, paged through delta";
            mainProgram = "godiff";
          };
        };
      });

      apps = forAllSystems (pkgs: {
        default = {
          type = "app";
          program = nixpkgs.lib.getExe self.packages.${pkgs.stdenv.hostPlatform.system}.godiff;
        };
      });

      devShells = forAllSystems (pkgs: {
        default = pkgs.mkShell {
          packages = with pkgs; [
            go
            gopls
            gofumpt
            golangci-lint
            git
            delta
            nixfmt-rfc-style
          ];
        };
      });

      checks = forAllSystems (pkgs: {
        godiff = self.packages.${pkgs.stdenv.hostPlatform.system}.godiff;
      });

      formatter = forAllSystems (pkgs: pkgs.nixfmt-rfc-style);
    };
}
