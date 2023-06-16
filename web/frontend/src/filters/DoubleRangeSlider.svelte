<!--
Copyright (c) 2021 Michael Keller
Originally created by Michael Keller (https://github.com/mhkeller/svelte-double-range-slider)
Changes: remove dependency, text inputs, configurable value ranges, on:change event
-->
<!-- 
	@component

	Properties:
	- min:          Number
	- max:          Number
	- firstSlider:  Number (Starting position of slider #1)
	- secondSlider: Number (Starting position of slider #2)
	Events:
	- `change`: [Number, Number] (Positions of the two sliders)
 -->
<script>
	import { createEventDispatcher } from "svelte";

	export let min;
	export let max;
	export let firstSlider;
	export let secondSlider;
	export let inputFieldFrom = 0;
	export let inputFieldTo = 0;

	const dispatch = createEventDispatcher();

	let values;
	let start, end; /* Positions of sliders from 0 to 1 */
	$: values = [firstSlider, secondSlider]; /* Avoid feedback loop */
	$: start = Math.max(((firstSlider  == null ? min : firstSlider)  - min) / (max - min), 0);
	$: end =   Math.min(((secondSlider == null ? min : secondSlider) - min) / (max - min), 1);

	let leftHandle;
	let body;
	let slider;

	let timeoutId = null;
	function queueChangeEvent() {
		if (timeoutId !== null) {
			clearTimeout(timeoutId);
		}

		timeoutId = setTimeout(() => {
			timeoutId = null;

			// Show selection but avoid feedback loop
			if (values[0] != null && inputFieldFrom != values[0].toString())
				inputFieldFrom = values[0].toString();
			if (values[1] != null && inputFieldTo != values[1].toString())
				inputFieldTo = values[1].toString();

			dispatch('change', values);
		}, 250);
	}

	function update() {
		values = [
			Math.floor(min + start  * (max - min)),
			Math.floor(min + end * (max - min))
		];
		queueChangeEvent();
	}

	function inputChanged(idx, event) {
		let val = Number.parseInt(event.target.value);
		if (Number.isNaN(val) || val < min) {
			event.target.classList.add('bad');
			return;
		}

		values[idx] = val;
		event.target.classList.remove('bad');
		if (idx == 0)
			start = clamp((val - min) / (max - min), 0., 1.);
		else
			end = clamp((val - min) / (max - min), 0., 1.);

		queueChangeEvent();
	}

	function clamp(x, min, max) {
		return x < min
			? min
			: (x < max ? x : max);
	}

	function draggable(node) {
		let x;
		let y;

		function handleMousedown(event) {
			if (event.type === 'touchstart') {
				event = event.touches[0];
			}
			x = event.clientX;
			y = event.clientY;

			node.dispatchEvent(new CustomEvent('dragstart', {
				detail: { x, y }
			}));

			window.addEventListener('mousemove', handleMousemove);
			window.addEventListener('mouseup', handleMouseup);

			window.addEventListener('touchmove', handleMousemove);
			window.addEventListener('touchend', handleMouseup);
		}

		function handleMousemove(event) {
			if (event.type === 'touchmove') {
				event = event.changedTouches[0];
			}

			const dx = event.clientX - x;
			const dy = event.clientY - y;

			x = event.clientX;
			y = event.clientY;

			node.dispatchEvent(new CustomEvent('dragmove', {
				detail: { x, y, dx, dy }
			}));
		}

		function handleMouseup(event) {
			x = event.clientX;
			y = event.clientY;

			node.dispatchEvent(new CustomEvent('dragend', {
				detail: { x, y }
			}));

			window.removeEventListener('mousemove', handleMousemove);
			window.removeEventListener('mouseup', handleMouseup);

			window.removeEventListener('touchmove', handleMousemove);
			window.removeEventListener('touchend', handleMouseup);
		}

		node.addEventListener('mousedown', handleMousedown);
		node.addEventListener('touchstart', handleMousedown);

		return {
			destroy() {
				node.removeEventListener('mousedown', handleMousedown);
				node.removeEventListener('touchstart', handleMousedown);
			}
		};
	}

	function setHandlePosition (which) {
		return function (evt) {
			const { left, right } = slider.getBoundingClientRect();
			const parentWidth = right - left;

			const p = Math.min(Math.max((evt.detail.x - left) / parentWidth, 0), 1);

			if (which === 'start') {
				start = p;
				end = Math.max(end, p);
			} else {
				start = Math.min(p, start);
				end = p;
			}

			update();
		}
	}

	function setHandlesFromBody (evt) {
		const { width } = body.getBoundingClientRect();
		const { left, right } = slider.getBoundingClientRect();

		const parentWidth = right - left;

		const leftHandleLeft = leftHandle.getBoundingClientRect().left;

		const pxStart = clamp((leftHandleLeft + evt.detail.dx) - left, 0, parentWidth - width);
		const pxEnd = clamp(pxStart + width, width, parentWidth);

		const pStart = pxStart / parentWidth;
		const pEnd = pxEnd / parentWidth;

		start = pStart;
		end = pEnd;
		update();
	}
</script>

<div class="double-range-container">
	<div class="header">
		<input class="form-control" type="text" placeholder="from..." bind:value={inputFieldFrom}
			on:input={(e) => inputChanged(0, e)} />

		<span>Full Range: <b> {min} </b> - <b> {max} </b></span>

		<input class="form-control" type="text" placeholder="to..." bind:value={inputFieldTo}
			on:input={(e) => inputChanged(1, e)} />
	</div>
	<div class="slider" bind:this={slider}>
		<div
			class="body"
			bind:this={body}
			use:draggable
			on:dragmove|preventDefault|stopPropagation="{setHandlesFromBody}"
			style="
				left: {100 * start}%;
				right: {100 * (1 - end)}%;
			"
			></div>
		<div
			class="handle"
			bind:this={leftHandle}
			data-which="start"
			use:draggable
			on:dragmove|preventDefault|stopPropagation="{setHandlePosition('start')}"
			style="
				left: {100 * start}%
			"
		></div>
		<div
			class="handle"
			data-which="end"
			use:draggable
			on:dragmove|preventDefault|stopPropagation="{setHandlePosition('end')}"
			style="
				left: {100 * end}%
			"
		></div>
	</div>
</div>

<style>
	.header {
		width: 100%;
		display: flex;
		justify-content: space-between;
		margin-bottom: -5px;
	}
	.header :nth-child(2) {
		padding-top: 10px;
	}
	.header input {
		height: 25px;
		border-radius: 5px;
		width: 100px;
	}

	:global(.double-range-container .header input[type="text"].bad) {
		color: #ff5c33;
		border-color: #ff5c33;
	}

	.double-range-container {
		width: 100%;
		height: 50px;
		user-select: none;
		box-sizing: border-box;
		white-space: nowrap
	}
	.slider {
		position: relative;
		width: 100%;
		height: 6px;
		top: 10px;
		transform: translate(0, -50%);
		background-color: #e2e2e2;
		box-shadow: inset 0 7px 10px -5px #4a4a4a, inset 0 -1px 0px 0px #9c9c9c;
		border-radius: 6px;
	}
	.handle {
		position: absolute;
		top: 50%;
		width: 0;
		height: 0;
	}
	.handle:after {
		content: ' ';
		box-sizing: border-box;
		position: absolute;
		border-radius: 50%;
		width: 16px;
		height: 16px;
		background-color: #fdfdfd;
		border: 1px solid #7b7b7b;
		transform: translate(-50%, -50%)
	}
	/* .handle[data-which="end"]:after{
		transform: translate(-100%, -50%);
	} */
	.handle:active:after {
		background-color: #ddd;
		z-index: 9;
	}
	.body {
		top: 0;
		position: absolute;
		background-color: #34a1ff;
		bottom: 0;
	}
</style>
