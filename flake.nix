{
  description = "pandaflix - terminal movie/TV streamer";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = { self, nixpkgs }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f system);
    in
    {
      packages = forAllSystems (system:
        let
          pkgs = nixpkgs.legacyPackages.${system};

          version = "1.2.1";

          binaryName = {
            "x86_64-linux"  = "pandaflix-linux-amd64";
            "aarch64-linux" = "pandaflix-linux-arm64";
            "x86_64-darwin" = "pandaflix-darwin-amd64";
            "aarch64-darwin" = "pandaflix-darwin-arm64";
          }.${system};

          sha256 = {
            "x86_64-linux"  = "sha256-cMjcxMMXtUYdzNDO7qh61hBtqgOgeNb5h5EOQHbf1hQ=";
            "aarch64-linux" = "sha256-br4VLn5FKLVN1L6HTqNuzifP0vGdmcUWj4El+mPK0dg=";
            "x86_64-darwin" = "sha256-ZRpyFI5lRODU5gD+PF2ZhoM9usdbBd1kel5e5H7E7Nc=";
            "aarch64-darwin" = "sha256-9Usl99kqljwEHSzwb/zhj41mywAV+QQkRklYUsw4Mio=";
          }.${system};

        in
        {
          pandaflix = pkgs.stdenvNoCC.mkDerivation {
            pname = "pandaflix";
            inherit version;

            src = pkgs.fetchurl {
              url = "https://github.com/DemonKingSwarn/pandaflix/releases/download/v${version}/${binaryName}";
              inherit sha256;
            };

            dontUnpack = true;
            dontBuild = true;

            nativeBuildInputs = [ pkgs.makeWrapper ];

            # runtime deps pandaflix needs
            propagatedBuildInputs = with pkgs; [ mpv fzf yt-dlp ffmpeg chafa ];

            installPhase = ''
              install -Dm755 $src $out/bin/pandaflix
            '';

            postFixup = ''
              wrapProgram $out/bin/pandaflix \
                --prefix PATH : ${pkgs.lib.makeBinPath (with pkgs; [ mpv fzf yt-dlp ffmpeg chafa ])}
            '';

            meta = with pkgs.lib; {
              description = "Spiritual successor of flix-cli and mov-cli";
              homepage    = "https://github.com/DemonKingSwarn/pandaflix";
              license     = licenses.gpl3Only;
              maintainers = [ ];
              platforms   = with platforms; linux ++ darwin;
              mainProgram = "pandaflix";
            };
          };

          default = self.packages.${system}.pandaflix;
        }
      );

      apps = forAllSystems (system: {
        default = {
          type    = "app";
          program = "${self.packages.${system}.pandaflix}/bin/pandaflix";
        };
      });
    };
}
