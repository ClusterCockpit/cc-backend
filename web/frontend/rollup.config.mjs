import svelte from 'rollup-plugin-svelte';
import replace from "@rollup/plugin-replace";
import commonjs from '@rollup/plugin-commonjs';
import resolve from '@rollup/plugin-node-resolve';
import terser from '@rollup/plugin-terser';
import css from 'rollup-plugin-css-only';

const production = !process.env.ROLLUP_WATCH;

const plugins = [
    svelte({
        compilerOptions: {
            // Enable run-time checks when not in production
            dev: !production,
            // Enable Svelte 5-specific features
            hydratable: true, // If using server-side rendering
            immutable: true, // Optimize updates for immutable data
            // As of sveltestrap 7.1.0, filtered warnings would appear for imported sveltestrap components
            warningFilter: (warning) => (
                warning.code !== 'element_invalid_self_closing_tag' &&
                warning.code !== 'a11y_interactive_supports_focus'
            )
        }
    }),

    // If you have external dependencies installed from
    // npm, you'll most likely need these plugins. In
    // some cases you'll need additional configuration -
    // consult the documentation for details:
    // https://github.com/rollup/plugins/tree/master/packages/commonjs
    resolve({
        browser: true,
        dedupe: ['svelte', '@sveltejs/kit'] // Ensure deduplication for Svelte 5
    }),
    commonjs(),

    // If we're building for production (npm run build
    // instead of npm run dev), minify
    production && terser(),

    replace({
        preventAssignment: true,
        values: {
            "process.env.NODE_ENV": JSON.stringify(production ? "production" : "development"),
        }
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
    ],
    watch: {
        clearScreen: false
    }
});

export default [
    entrypoint('header', 'src/header.entrypoint.js'),
    entrypoint('jobs', 'src/jobs.entrypoint.js'),
    entrypoint('user', 'src/user.entrypoint.js'),
    entrypoint('list', 'src/list.entrypoint.js'),
    entrypoint('taglist', 'src/tags.entrypoint.js'),
    entrypoint('job', 'src/job.entrypoint.js'),
    entrypoint('systems', 'src/systems.entrypoint.js'),
    entrypoint('node', 'src/node.entrypoint.js'),
    entrypoint('analysis', 'src/analysis.entrypoint.js'),
    entrypoint('status', 'src/status.entrypoint.js'),
    entrypoint('config', 'src/config.entrypoint.js')
];
