// Load plugins
const browsersync = require("browser-sync").create();
const gulp = require("gulp");
const exec = require("gulp-exec");

// Copy third party libraries from /node_modules into /vendor
gulp.task('vendor', function(cb) {
  // Ace
  gulp.src([
      './node_modules/ace-builds/src-min-noconflict/ace.js',
      './node_modules/ace-builds/src-min-noconflict/mode-yaml.js',
      './node_modules/ace-builds/src-min-noconflict/mode-text.js',
      './node_modules/ace-builds/src-min-noconflict/theme-monokai.js'
    ])
    .pipe(gulp.dest('./vendor/ace'))

  cb();

});

// BrowserSync
function browserSync(done) {
  browsersync.init({
    server: {
      baseDir: "./"
    }
  });
  done();
}

// BrowserSync Reload
function browserSyncReload(done) {
  browsersync.reload();
  done();
}

// Watch files
function watchFiles() {
  gulp.watch("./css/*", browserSyncReload);
  gulp.watch("./**/*.html", browserSyncReload);
}

gulp.task("default", gulp.parallel('vendor'));

// dev task
gulp.task("dev", gulp.parallel(watchFiles, browserSync));

gulp.task("wasm", function() {
  var options = {
    pipeStdout: false, // default = false, true means stdout is written to file.contents
  };
  var reportOptions = {
  	err: true, // default = true, false means don't write err
  	stderr: true, // default = true, false means don't write stderr
  	stdout: true // default = true, false means don't write stdout
  };
  return gulp.src('./wasm/main.go')
    .pipe(exec('GOOS=js GOARCH=wasm go build -o wasm/benthos-lab.wasm <%= file.path %>', options))
    .pipe(exec.reporter(reportOptions));
});
