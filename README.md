> ⚠️ **DEVELOPMENT PREVIEW NOTICE**
>
> Vulpix is currently in development preview. While we are actively working on improving its stability and features, please be aware that you may encounter bugs or incomplete functionalities. We appreciate your patience and feedback as we continue to develop and refine Vulpix.
>
> This software is not yet recommended for production use. Use at your own risk.

<p align="center">
  <a href="https://github.com/vulpeslab/vulpix">
    <picture>
      <source srcset="./assets/vulpix-dark.png" media="(prefers-color-scheme: dark)">
      <source srcset="./assets/vulpix-light.png" media="(prefers-color-scheme: light)">
      <img src="./assets/vulpix-light.png" alt="Vulpix logo" width="80%">
    </picture>
  </a>
</p>

---

[Vulpix](https://github.com/vulpeslab/vulpix) is a terminal-based AI assistant that leverages large language models to provide a seamless and efficient user experience. It is designed to be lightweight, fast, and highly customizable, making it an ideal tool for developers, writers, and anyone looking to enhance their productivity with AI.

## Installation

> ⚠️ **Important Note:** As Vulpix is in active development, the only supported installation method is manually building from source. Pre-built binaries and automated installation scripts may not be up-to-date with the latest changes. We recommend following the manual build instructions below to ensure you have the most recent version.

### Manual Build

To build Vulpix from source, ensure you have [Go](https://go.dev/dl/) installed (version 1.25.4 or higher is recommended). Then, clone the repository and build the project:

```bash
# Clone the repository
git clone https://github.com/vulpeslab/vulpix

# Navigate to the project directory
cd vulpix

# Build the project (macOS/Linux)
go build -o vulpix ./cmd/vulpix

# Build the project (Windows)
go build -o vulpix.exe ./cmd/vulpix
```

Move the resulting binary to a directory in your PATH for global access, or run it directly from the build directory using `./vulpix` (Linux/macOS) or `.\vulpix.exe` (Windows).

## License

Vulpix is licensed under the [Apache License 2.0](./LICENSE).
