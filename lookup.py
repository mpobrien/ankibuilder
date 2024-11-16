from wrpy import WordReference
from termcolor import colored
import json
import sys
from airium import Airium


wr = WordReference("en", "es");

"""
response {}
    ├ word
    ├ from_lang
    ├ to_lang
    ├ url  # hitted url
    └ translations []
        ├ title  # title of each section
        └ entries []
            ├ context
            ├ from_word {}
            │    ├ source   # source word
            │    └ grammar  # grammar tips about source word
            ├ to_word []
            │    ├ meaning
            │    ├ notes    # clarification about meaning
            │    └ grammar  # grammar tips about meaning
            ├ from_example
            └ to_example []
"""

def html_es_en(front, back, example):
    a = Airium()
    with a.body():
        with a.b():
            a("Hello World.")

    html_text = str(a)  # casting to string extracts the value
# or
    html_bytes = bytes(a)  # generates the same but encoded with UTF-8

    print(html_text)


    print("<html>"

    )

def prettyprint_result(result):
    for translation in result['translations']:
        for i, entry in enumerate(translation['entries']):
            print(
                colored(entry['from_word']['source'], "red", attrs=["bold"]),
                "("+colored(entry['from_word']['grammar'], "light_grey", attrs=[])+")",
            )
            for to_word in entry['to_word']:
                print(
                    "\t\t",
                    colored(to_word['meaning'], "yellow", attrs=["bold"]),
                )
                print(
                    "\t\t",
                    colored(entry['context'], "green", attrs=[]),
                )
            print()

def main(args):
    if len(args) < 2:
        print("Usage: ", args[0], "<word>")
        return
    inputword = args[1]
    # Echo the line back
    try:
        translation = wr.translate(inputword)
        prettyprint_result(translation)
    except NameError:
        print("No translation found for", inputword)



import json
import urllib.request

def request(action, **params):
    return {'action': action, 'params': params, 'version': 6}

def invoke(action, **params):
    requestJson = json.dumps(request(action, **params)).encode('utf-8')
    response = json.load(urllib.request.urlopen(urllib.request.Request('http://127.0.0.1:8765', requestJson)))
    if len(response) != 2:
        raise Exception('response has an unexpected number of fields')
    if 'error' not in response:
        raise Exception('response is missing required error field')
    if 'result' not in response:
        raise Exception('response is missing required result field')
    if response['error'] is not None:
        raise Exception(response['error'])
    return response['result']

if __name__ == "__main__":
    main(sys.argv)

