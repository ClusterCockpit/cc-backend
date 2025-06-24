<!--
Copyright (c) 2021 Michael Keller
Originally created by Michael Keller (https://github.com/mhkeller/svelte-double-range-slider)
Changes: remove dependency, text inputs, configurable value ranges, on:change event
Changes #2: Rewritten for Svelte 5, removed bodyHandler
-->
<!-- 
	@component Selector component to display range selections via min and max double-sliders

	Properties:
	- min:          Number
	- max:          Number
	- sliderHandleFrom:  Number (Starting position of slider #1)
	- sliderHandleTo: Number (Starting position of slider #2)
	
	Events:
	- `change`: [Number, Number] (Positions of the two sliders)
 -->

<script>
	let {
		sliderMin,
		sliderMax,
		fromPreset = 1,
		toPreset = 100,
		changeRange
	} = $props();

	let pendingValues = $state([fromPreset, toPreset]);
	let sliderFrom = $state(Math.max(((fromPreset  == null ? sliderMin : fromPreset)  - sliderMin) / (sliderMax - sliderMin), 0.));
	let sliderTo = $state(Math.min(((toPreset == null ? sliderMin : toPreset) - sliderMin) / (sliderMax - sliderMin), 1.));
	let inputFieldFrom = $state(fromPreset.toString());
	let inputFieldTo = $state(toPreset.toString());
	let leftHandle = $state();
	let sliderMain = $state();

	let timeoutId = null;
	function queueChangeEvent() {
		if (timeoutId !== null) {
			clearTimeout(timeoutId)
		}
		timeoutId = setTimeout(() => {
			timeoutId = null
			changeRange(pendingValues);
		}, 100);
	}

	function updateStates(newValue, newPosition, target) {
		if (target === 'from') {
			pendingValues[0] = isNaN(newValue) ? null : newValue;
			inputFieldFrom = isNaN(newValue) ? null : newValue.toString();
			sliderFrom = newPosition;
		} else if (target === 'to') {
			pendingValues[1] = isNaN(newValue) ? null : newValue;
			inputFieldTo = isNaN(newValue) ? null : newValue.toString();
			sliderTo = newPosition;
		}

		queueChangeEvent();
	}

	function rangeChanged (evt, target) {
		evt.preventDefault()
		evt.stopPropagation()
		const { left, right } = sliderMain.getBoundingClientRect();
		const parentWidth = right - left;
		const newP = Math.min(Math.max((evt.detail.x - left) / parentWidth, 0), 1);
		const newV = Math.floor(sliderMin + newP * (sliderMax - sliderMin));
		updateStates(newV, newP, target);
	}

	function inputChanged(evt, target) {
		evt.preventDefault()
		evt.stopPropagation()
		const newV = Number.parseInt(evt.target.value);
		const newP = clamp((newV - sliderMin) / (sliderMax - sliderMin), 0., 1.)
		updateStates(newV, newP, target);
	}

	function clamp(x, testMin, testMax) {
		return x < testMin
			? testMin
			: (x > testMax 
				? testMax
				: x
			);
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

</script>

<div class="double-range-container">
	<div class="header">
		<input class="form-control" type="text" placeholder="from..." value={inputFieldFrom}
			oninput={(e) => inputChanged(e, 'from')} />

		<span>Full Range: <b> {sliderMin} </b> - <b> {sliderMax} </b></span>

		<input class="form-control" type="text" placeholder="to..." value={inputFieldTo}
			oninput={(e) => inputChanged(e, 'to')} />
	</div>

	<div id="slider-active" class="slider" bind:this={sliderMain}>
		<div
			class="slider-body"
			style="left: {100 * sliderFrom}%;right: {100 * (1 - sliderTo)}%;"
			></div>
		<div
			class="slider-handle"
			bind:this={leftHandle}
			data-which="from"
			use:draggable
			ondragmove={(e) => rangeChanged(e, 'from')}
			style="left: {100 * sliderFrom}%"
		></div>
		<div
			class="slider-handle"
			data-which="to"
			use:draggable
			ondragmove={(e) => rangeChanged(e, 'to')}
			style="left: {100 * sliderTo}%"
		></div>
	</div>
</div>

<style>
	.header {
		width: 100%;
		display: flex;
		justify-content: space-between;
		align-items: flex-end;
		margin-bottom: 5px;
	}
	.header :nth-child(2) {
		padding-top: 10px;
	}
	.header input {
		height: 25px;
		border-radius: 5px;
		width: 100px;
	}

	.double-range-container {
		width: 100%;
		height: 50px;
		user-select: none;
		box-sizing: border-box;
		white-space: nowrap;
		margin-top: -4px;
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
	.slider-handle {
		position: absolute;
		top: 50%;
		width: 0;
		height: 0;
	}
	.slider-handle:after {
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
	.slider-handle:active:after {
		background-color: #ddd;
		z-index: 9;
	}
	.slider-body {
		top: 0;
		position: absolute;
		background-color: #34a1ff;
		bottom: 0;
	}
</style>
