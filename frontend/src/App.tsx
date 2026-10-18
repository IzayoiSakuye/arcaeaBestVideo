import { type SetStateAction, useState} from 'react';
import {Version} from "../wailsjs/go/app/App";

function App() {
    const [resultText, setResultText] = useState("Please enter your name below 👇");
    const [_, setName] = useState('');
    const updateName = (e: { target: { value: string; }; }) => setName(e.target.value);
    const updateResultText = (result: SetStateAction<string>) => setResultText(result);

    function version() {
        Version().then(updateResultText);
    }

    return (
        <div id="App">
            <div id="result" className="result">{resultText}</div>
            <div id="input" className="input-box">
                <input id="name" className="input" onChange={updateName} autoComplete="off" name="input" type="text"/>
                <button className="btn" onClick={version}>Version</button>
            </div>
        </div>
    )
}

export default App
