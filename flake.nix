{
  description = "behtree - behaviour tree library";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in
      {
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go

            # Formatting
            gofumpt          # Stricter gofmt

            # Metrics & linting
            golangci-lint    # Aggregator: gocyclo, gocognit, dupl, funlen, maintidx, goconst
            gocyclo          # Cyclomatic complexity (standalone)
            goconst          # Repeated strings/numbers
            scc              # LoC, blanks, comments, complexity, COCOMO, DRYness
          ];

          shellHook = ''
            echo "behtree dev shell ready"
            echo "  make fmt       - format code"
            echo "  make metrics   - run all metrics"
            echo "  make lint      - run golangci-lint"
          '';
        };
      });
}
