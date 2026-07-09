{
  description = "Build environment for qjs.wasm (QuickJS WASM sandbox)";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs?rev=4b80ef8d72d7341a04de3fe4ac1f0fb58e630053";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs =
    {
      self,
      nixpkgs,
      flake-utils,
    }:
    flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = import nixpkgs { inherit system; };
        wasi-sdk = pkgs.fetchzip rec {
          name = "wasi-sdk-${version}-${system}";
          version = "24.0";
          url = "https://github.com/WebAssembly/wasi-sdk/releases/download/wasi-sdk-24/wasi-sdk-${version}-x86_64-linux.tar.gz";
          hash = "sha256-/cyLxhFsfBBQxn4NrhLdbgHjU3YUjYhPnvquWJodcO8=";

        };
      in
      {
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            cmake
            binaryen
          ];

          shellHook = ''
            export WASI_SDK=${wasi-sdk}
            export TOOLCHAIN_FILE=${wasi-sdk}/share/cmake/wasi-sdk.cmake
          '';
        };
      }
    );
}
