/*
    Collect Functions for Unit Handling and Scaling Here
*/

const power  = [1, 1e3, 1e6, 1e9, 1e12, 1e15, 1e18, 1e21]
const prefix = ['', 'K', 'M', 'G', 'T', 'P', 'E']

export function formatNumber(x) {
    if ( isNaN(x) ) {
        return x // Return if String , used in Histograms
    } else {
        for (let i = 0; i < prefix.length; i++)
            if (power[i] <= x && x < power[i+1])
                return `${Math.round((x / power[i]) * 100) / 100} ${prefix[i]}`

        return Math.abs(x) >= 1000 ? x.toExponential() : x.toString()
    }
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

