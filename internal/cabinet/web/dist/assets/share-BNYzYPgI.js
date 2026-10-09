import{i as s,ar as c,au as p}from"./index-Bn0G99w1.js";/**
 * @license lucide-react v0.475.0 - ISC
 *
 * This source code is licensed under the ISC license.
 * See the LICENSE file in the root directory of this source tree.
 */const l=[["path",{d:"M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4",key:"ih7n3h"}],["polyline",{points:"17 8 12 3 7 8",key:"t8dd8p"}],["line",{x1:"12",x2:"12",y1:"3",y2:"15",key:"widbto"}]],m=s("Upload",l);function i(n,e){return`https://t.me/share/url?url=${encodeURIComponent(e)}&text=${encodeURIComponent(n)}`}function u(n){try{const e=window.open(n,"_blank");if(!e)return!1;try{e.opener=null}catch{}return!0}catch{return!1}}async function h({text:n,url:e}){var o;const t=e==null?void 0:e.trim();if(!t)return!1;const r=(n==null?void 0:n.trim())??"";if(c()){const a=(o=window.Telegram)==null?void 0:o.WebApp;if(a&&typeof a.openTelegramLink=="function")try{return a.openTelegramLink(i(r,t)),!0}catch{}}if(p()&&typeof navigator.share=="function")try{return await navigator.share({text:`${r}
${t}`.trim()}),!0}catch{return!1}return u(i(r,t))}export{m as U,h as s};
