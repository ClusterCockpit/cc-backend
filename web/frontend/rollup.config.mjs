import svelte from 'rollup-plugin-svelte';
import replace from "@rollup/plugin-replace";
import commonjs from '@rollup/plugin-commonjs';
import resolve from '@rollup/plugin-node-resolve';
import terser from '@rollup/plugin-terser';
import css from 'rollup-plugin-css-only';
import livereload from 'rollup-plugin-livereload';

// const production = !process.env.ROLLUP_WATCH;
const production = false

const plugins = [
    svelte({
        compilerOptions: {
            // enable run-time checks when not in production
            dev: !production
        }
    }),

    // If you have external dependencies installed from
    // npm, you'll most likely need these plugins. In
    // some cases you'll need additional configuration -
    // consult the documentation for details:
    // https://github.com/rollup/plugins/tree/master/packages/commonjs
    resolve({
        browser: true,
        dedupe: ['svelte']
    }),
    commonjs(),

    // If we're building for production (npm run build
    // instead of npm run dev), minify
    production && terser(),

    replace({
        "process.env.NODE_ENV": JSON.stringify("development"),
        preventAssignment: true
    })
];

const entrypoint = (name, path) => ({
    input: path,
    output: {
        sourcemap: false,
        format: 'iife',
        name: 'app',
        file: `public/build/${name}.js`
    },
    plugins: [
        ...plugins,

        // we'll extract any component CSS out into
        // a separate file - better for performance
        css({ output: `${name}.css` }),
        // livereload('public')
    ],
    watch: {
        clearScreen: false
    }
});

export default [
    entrypoint('header', 'src/header.entrypoint.js'),
    entrypoint('home', 'src/home.entrypoint.js'),
    // entrypoint('jobs', 'src/jobs.entrypoint.js'),
    // entrypoint('user', 'src/user.entrypoint.js'),
    // entrypoint('list', 'src/list.entrypoint.js'),
    // entrypoint('job', 'src/job.entrypoint.js'),
    // entrypoint('systems', 'src/systems.entrypoint.js'),
    entrypoint('node', 'src/node.entrypoint.js'),
    // entrypoint('analysis', 'src/analysis.entrypoint.js'),
    entrypoint('control', 'src/control.entrypoint.js'),
    entrypoint('config', 'src/config.entrypoint.js'),
    entrypoint('partitions', 'src/partitions.entrypoint.js'),
    entrypoint('history', 'src/history.entrypoint.js')

];
