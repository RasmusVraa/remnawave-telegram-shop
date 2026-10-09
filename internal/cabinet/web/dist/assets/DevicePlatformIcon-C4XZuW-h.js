import{j as o}from"./query-COvk_SeJ.js";import{i as r,r as c,a7 as n}from"./index-Bn0G99w1.js";import{T as s}from"./tv-BYTZtE2h.js";import{L as u}from"./laptop-R_93XBKl.js";/**
 * @license lucide-react v0.475.0 - ISC
 *
 * This source code is licensed under the ISC license.
 * See the LICENSE file in the root directory of this source tree.
 */const a=[["rect",{width:"20",height:"14",x:"2",y:"3",rx:"2",key:"48i651"}],["line",{x1:"8",x2:"16",y1:"21",y2:"21",key:"1svkeh"}],["line",{x1:"12",x2:"12",y1:"17",y2:"21",key:"vw1qmm"}]],d=r("Monitor",a);/**
 * @license lucide-react v0.475.0 - ISC
 *
 * This source code is licensed under the ISC license.
 * See the LICENSE file in the root directory of this source tree.
 */const l=[["rect",{width:"16",height:"20",x:"4",y:"2",rx:"2",ry:"2",key:"76otgf"}],["line",{x1:"12",x2:"12.01",y1:"18",y2:"18",key:"1dp563"}]],m=r("Tablet",l);function x(i){return(i==null?void 0:i.toLowerCase().trim())??""}function y(i){const e=x(i);return e.includes("tv")?s:e.includes("ipad")||e.includes("tablet")?m:e.includes("iphone")||e.includes("ios")||e.includes("android")?n:e.includes("mac")||e.includes("darwin")?u:e.includes("windows")||e.includes("linux")?d:n}function w({platform:i,className:e}){const t=y(i);return o.jsx(t,{className:c("size-4",e),"aria-hidden":!0})}export{w as D};
