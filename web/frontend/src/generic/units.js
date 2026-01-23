/*
    Collect Functions for Unit Handling and Scaling Here
*/

const power  = [1, 1e3, 1e6, 1e9, 1e12, 1e15, 1e18, 1e21]
const prefix = ['', 'k', 'M', 'G', 'T', 'P', 'E']

export function formatNumber(x) {
    if ( isNaN(x) || x == null) {
        return x // Return if String or Null
    } else {
        for (let i = 0; i < prefix.length; i++)
            if (power[i] <= x && x < power[i+1])
                return `${Math.round((x / power[i]) * 100) / 100} ${prefix[i]}`

        return Math.abs(x) >= 1000 ? x.toExponential() : x.toString()
    }
}

export function roundTwoDigits(x) {
    return Math.round(x * 100) / 100
}

export function scaleNumbers(x, y , p = '') {
    const oldPower  = power[prefix.indexOf(p)]
    const rawXValue = x * oldPower 
    const rawYValue = y * oldPower 

    for (let i = 0; i < prefix.length; i++) {
        if (power[i] <= rawYValue && rawYValue < power[i+1]) {
            return `${Math.round((rawXValue / power[i]) * 100) / 100} / ${Math.round((rawYValue / power[i]) * 100) / 100} ${prefix[i]}`
        }
    }

    return Math.abs(rawYValue) >= 1000 ? `${rawXValue.toExponential()} / ${rawYValue.toExponential()}` : `${rawYValue.toString()} / ${rawYValue.toString()}`
}

export function formatDurationTime(t, forNode = false) {
    if (t !== null) {
        if (isNaN(t)) {
            return t;
        } else {
            const tAbs = Math.abs(t);
            const h = Math.floor(tAbs / 3600);
            const m = Math.floor((tAbs % 3600) / 60);
            // Re-Add "negativity" to time ticks only as string, so that if-cases work as intended
            if (h == 0) return `${forNode && m != 0 ? "-" : ""}${m}m`;
            else if (m == 0) return `${forNode ? "-" : ""}${h}h`;
            else return `${forNode ? "-" : ""}${h}:${m}h`;
        }
    }
}

export function formatUnixTime(t, withDate = false) {
    if (t !== null) {
        if (isNaN(t)) {
            return t;
        } else {
            if (withDate) return new Date(t * 1000).toLocaleString();
            else return new Date(t * 1000).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
        }
    }
}

// const equalsCheck = (a, b) => {
//   return JSON.stringify(a) === JSON.stringify(b);
// }

// export const dateToUnixEpoch = (rfc3339) => Math.floor(Date.parse(rfc3339) / 1000);
