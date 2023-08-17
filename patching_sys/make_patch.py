# Copyright (C) 2023 crazybie@github.com.

import argparse
import datetime
import json
import os
import random
import re
import shutil
import subprocess
import time
import traceback

AParser = argparse.ArgumentParser()
AParser.add_argument("--output_dir", "-o", default=".")
AParser.add_argument("--debug", "-d", default="")
AParser.add_argument("--watch_dir", "-w", default=".")
AParser.add_argument("--input_dir", "-i")
AParser.add_argument("--tags", default="gdb")
AParser.add_argument("--clone_tmp", default="/tmp")
app_args = AParser.parse_args()

debug = app_args.debug


# Pattern: patching_sys.Register[PlayerObj]()
def scan_types(folder):
    types = set()
    files = []
    reg_files = []
    for i in os.listdir(folder):
        if i.endswith('.go'):
            full = os.path.abspath(os.path.join(folder, i))
            d = open(full).read()
            reg_types = list(re.findall(r'\s+patching_sys\.Register\[(.*)\]\(\)', d))
            types.update(reg_types)
            if reg_types:
                reg_files.append(full)
            files.append((full, d))

    print('discovered types:')
    print('------')
    for i in types:
        print(i)
    print('------')
    return files, types, reg_files


def clone_package(folder, files, ver):
    new_folder = f'{app_args.clone_tmp}/{folder}_{ver}'
    shutil.rmtree(new_folder, ignore_errors=True)
    os.makedirs(new_folder)
    new_files = []
    replace = {}
    for full, d in files:
        f2 = os.path.abspath(os.path.join(new_folder, os.path.basename(full)))
        new_files.append((f2, d))
        replace[full] = f2

    overlay = os.path.join(new_folder, 'overlay.json')
    open(overlay, 'w').write(json.dumps({'Replace': replace}, indent=1))
    if debug:
        print('write overlay', os.path.abspath(overlay))
    return new_folder, new_files, overlay, replace


def replace_package(files):
    pkg = ''
    for full, d in files:
        m = re.search(r'package\s+(\w+)', d)
        if not m:
            raise Exception('file has no package: %s' % full)

        p = m.group(1)
        if pkg and p != pkg:
            raise Exception("multiple packages: %s, %s", pkg, p)
        pkg = p
        if pkg != 'main':
            c = re.sub(r'package %s' % pkg, 'package main', d)
            open(full, 'w').write(c)
    if debug:
        print('replace pkg')
    return pkg


def code_gen(out_file, types, ver):
    new_types = ''
    reg_types = ''

    for i in types:
        new_types += f'''    
        
func (s *{i})Ver__() string {{
    return "{ver}"
}}

type SoTmp{ver}_{i} struct {{
    {i}
}}

    '''
        reg_types += f'''
    f.RegisterTp((*SoTmp{ver}_{i})(nil))
    '''

    so_main = f'''

type TmpFactory{ver} struct{{
    patching_sys.SoTypeFactory
}}

var tmpFactory TmpFactory{ver}

func GetFactory() patching_sys.Factory {{
    return &tmpFactory
}}

{new_types}

func (f *TmpFactory{ver}) Init() string {{  
    f.SoTypeFactory.Reset()
    {reg_types}
    return "{ver}"
}}
    '''

    open(out_file, 'a').write(so_main)


def build_so(folder, overlay, so_output):
    print('building', so_output)
    abs_out_dir = os.path.abspath(app_args.output_dir)
    cwd = os.getcwd()
    os.chdir(folder)
    try:
        s_tm = datetime.datetime.now()
        full = f'{abs_out_dir}/{so_output}'
        args = ['go', 'build', '-buildmode=plugin', f'-o={full}', f'-tags={app_args.tags}',
                '-gcflags=all=-N -l']
        if overlay:
            args.append(f'-overlay={overlay}')
        if debug: print(args)
        subprocess.check_call(args)
        print('took', datetime.datetime.now() - s_tm)
        return full
    finally:
        os.chdir(cwd)


def cleanup(new_folder):
    shutil.rmtree(new_folder, ignore_errors=True)
    if debug:
        print('cleaned up')


class DirWatcher:
    def __init__(self, folder, suffix, cb):
        self.times = {}
        self.folder = folder
        self.suffix = suffix
        self.cb = cb
        self.read_times()

    def iter_folder(self):
        for root, dirs, files in os.walk(self.folder):
            for f in files:
                if f.endswith(self.suffix):
                    full = os.path.join(root, f)
                    yield full, os.path.getmtime(full)

    def read_times(self):
        for full, mtime in self.iter_folder():
            self.times[full] = mtime

    def run(self):
        print('watching dir', self.folder)
        while True:
            for full, mtime in self.iter_folder():
                if full not in self.times or mtime > self.times[full]:
                    try:
                        self.cb(full)
                    except:
                        traceback.print_exc()
                    self.read_times()
            time.sleep(1)


def file_changed(f):
    print('----------------------')
    print('file changed', os.path.relpath(f, app_args.watch_dir))
    folder = os.path.dirname(f)
    handle_folder(folder)


last_so = ''


def handle_folder(folder):
    ver = f'{random.randint(0, 100000)}'
    global last_so
    last_so = ''
    new_folder = []
    try:
        files, types, reg_files = scan_types(folder)
        if not types:
            print('no types discovered')
            return

        new_folder, files, overlay, replace = clone_package(folder, files, ver)
        pkg = replace_package(files)
        code_gen(replace[reg_files[0]], types, ver)
        last_so = build_so(folder, overlay, f'{pkg}_{ver}.so')
    finally:
        cleanup(new_folder)


if app_args.input_dir:
    handle_folder(app_args.input_dir)
else:
    try:
        w = DirWatcher(app_args.watch_dir, ".go", file_changed)
        w.run()
    finally:
        if os.path.isfile(last_so):
            os.remove(last_so)
