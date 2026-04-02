const webpack = require('webpack');

module.exports = {
  webpack: {
    configure: (webpackConfig) => {
      // Add fallbacks for Node.js modules
      webpackConfig.resolve.fallback = {
        ...webpackConfig.resolve.fallback,
        "http": require.resolve("stream-http"),
        "https": require.resolve("https-browserify"),
        "url": require.resolve("url/"),
        "buffer": require.resolve("buffer"),
        "crypto": require.resolve("crypto-browserify"),
        "stream": require.resolve("stream-browserify"),
        "assert": require.resolve("assert/"),
        "os": require.resolve("os-browserify/browser"),
        "path": require.resolve("path-browserify"),
        "zlib": require.resolve("browserify-zlib"),
        "util": require.resolve("util/"),
        "vm": require.resolve("vm-browserify"),
        "process/browser": require.resolve("process/browser")
      };

      // Add alias for process/browser
      webpackConfig.resolve.alias = {
        ...webpackConfig.resolve.alias,
        "process/browser": require.resolve("process/browser")
      };

      // Add plugins
      webpackConfig.plugins = [
        ...webpackConfig.plugins,
        new webpack.ProvidePlugin({
          process: 'process/browser',
          Buffer: ['buffer', 'Buffer'],
        }),
      ];

      // Ignore source map warnings
      webpackConfig.ignoreWarnings = [
        /Failed to parse source map/,
        /Critical dependency: the request of a dependency is an expression/
      ];

      // Configure source map loader to ignore missing source maps
      const sourceMapLoaderRule = webpackConfig.module.rules.find(
        rule => rule.enforce === 'pre' && rule.use && rule.use.some(
          use => use.loader && use.loader.includes('source-map-loader')
        )
      );
      
      if (sourceMapLoaderRule) {
        sourceMapLoaderRule.exclude = [
          /node_modules\/@fractalwagmi/,
          /node_modules\/eth-rpc-errors/,
          /node_modules\/jsbi/,
          /node_modules\/@metamask/,
          /node_modules\/superstruct/
        ];
      }

      return webpackConfig;
    },
  },
  devServer: {
    allowedHosts: 'all',
    host: '0.0.0.0',
    port: 3001,
    hot: true,
    liveReload: true,
    client: {
      webSocketURL: 'auto://0.0.0.0:0/ws',
      overlay: {
        errors: true,
        warnings: false,
      },
    },
    static: {
      directory: require('path').join(__dirname, 'public'),
      publicPath: '/',
    },
    historyApiFallback: {
      disableDotRule: true,
    },
  },
}; 